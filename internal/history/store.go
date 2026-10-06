// Package history persists usage samples and learns real window lengths.
package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/4ster-light/go-pane/internal/metrics"
	"github.com/4ster-light/go-pane/internal/opencode"
)

// Sample is one persisted poll.
type Sample struct {
	Time     time.Time            `json:"time"`
	Percents map[string]float64   `json:"percents"`
	ResetsAt map[string]time.Time `json:"resetsAt"`
}

type learnedFile struct {
	Durations map[string]int64 `json:"durationsSeconds"`
}

// Store is an append-only JSONL history with learned window lengths.
type Store struct {
	dir     string
	mu      sync.Mutex
	last    map[metrics.Kind]time.Time
	lengths metrics.Lengths
}

// Open loads (or initializes) the store under dir.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{
		dir:     dir,
		last:    map[metrics.Kind]time.Time{},
		lengths: metrics.DefaultLengths(),
	}
	s.loadLengths()
	s.loadLastSample()
	s.trim()
	return s, nil
}

func (s *Store) historyPath() string { return filepath.Join(s.dir, "history.jsonl") }
func (s *Store) lengthsPath() string { return filepath.Join(s.dir, "windows.json") }

func (s *Store) loadLengths() {
	b, err := os.ReadFile(s.lengthsPath())
	if err != nil {
		return
	}
	var lf learnedFile
	if json.Unmarshal(b, &lf) != nil {
		return
	}
	for k, secs := range lf.Durations {
		if secs <= 0 {
			continue
		}
		kind := metrics.Kind(k)
		s.lengths.Durations[kind] = time.Duration(secs) * time.Second
		s.lengths.Estimated[kind] = false
	}
}

func (s *Store) saveLengthsLocked() {
	out := learnedFile{Durations: map[string]int64{}}
	for k, d := range s.lengths.Durations {
		if s.lengths.Estimated[k] {
			continue
		}
		out.Durations[string(k)] = int64(d / time.Second)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return
	}
	tmp := s.lengthsPath() + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, s.lengthsPath())
	}
}

func (s *Store) loadLastSample() {
	f, err := os.Open(s.historyPath())
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var last Sample
	found := false
	for sc.Scan() {
		var smp Sample
		if json.Unmarshal(sc.Bytes(), &smp) == nil {
			last = smp
			found = true
		}
	}
	if !found {
		return
	}
	for k, t := range last.ResetsAt {
		s.last[metrics.Kind(k)] = t
	}
}

// Record appends a sample and learns window lengths when resets advance.
func (s *Store) Record(now time.Time, u opencode.Usage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	smp := Sample{
		Time:     now.UTC(),
		Percents: map[string]float64{},
		ResetsAt: map[string]time.Time{},
	}
	learnedChanged := false
	for _, k := range metrics.Order {
		w := windowFor(k, u)
		smp.Percents[string(k)] = w.Percent
		smp.ResetsAt[string(k)] = w.ResetsAt.UTC()

		prev, ok := s.last[k]
		if ok && !prev.IsZero() && w.ResetsAt.After(prev) {
			d := w.ResetsAt.Sub(prev)
			if plausibleWindow(k, d) && (s.lengths.Durations[k] != d || s.lengths.Estimated[k]) {
				s.lengths.Durations[k] = d
				s.lengths.Estimated[k] = false
				learnedChanged = true
			}
		}
		s.last[k] = w.ResetsAt
	}
	if learnedChanged {
		s.saveLengthsLocked()
	}

	f, err := os.OpenFile(s.historyPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	enc, _ := json.Marshal(smp)
	enc = append(enc, '\n')
	_, _ = f.Write(enc)
}

// Lengths returns a copy of the learned window lengths.
func (s *Store) Lengths() metrics.Lengths {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := metrics.Lengths{
		Durations: map[metrics.Kind]time.Duration{},
		Estimated: map[metrics.Kind]bool{},
	}
	for k, v := range s.lengths.Durations {
		out.Durations[k] = v
		out.Estimated[k] = s.lengths.Estimated[k]
	}
	return out
}

// Recent returns up to limit samples, oldest first.
func (s *Store) Recent(limit int) []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.historyPath())
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var all []Sample
	for sc.Scan() {
		var smp Sample
		if json.Unmarshal(sc.Bytes(), &smp) == nil {
			all = append(all, smp)
		}
	}
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all
}

func (s *Store) trim() {
	fi, err := os.Stat(s.historyPath())
	if err != nil || fi.Size() < 8<<20 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.Open(s.historyPath())
	if err != nil {
		return
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines [][]byte
	for sc.Scan() {
		lines = append(lines, append([]byte(nil), sc.Bytes()...))
	}
	f.Close()
	if len(lines) <= 10000 {
		return
	}
	lines = lines[len(lines)-10000:]
	tmp := s.historyPath() + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return
	}
	for _, l := range lines {
		_, _ = out.Write(l)
		_, _ = out.Write([]byte{'\n'})
	}
	out.Close()
	_ = os.Rename(tmp, s.historyPath())
}

// plausibleWindow guards against learning a bogus length when the daemon was
// offline across several resets.
func plausibleWindow(k metrics.Kind, d time.Duration) bool {
	switch k {
	case metrics.Rolling:
		return d >= 1*time.Hour && d <= 12*time.Hour
	case metrics.Weekly:
		return d >= 6*24*time.Hour && d <= 8*24*time.Hour
	case metrics.Monthly:
		return d >= 25*24*time.Hour && d <= 35*24*time.Hour
	}
	return false
}

func windowFor(k metrics.Kind, u opencode.Usage) opencode.Window {
	switch k {
	case metrics.Rolling:
		return u.Rolling
	case metrics.Weekly:
		return u.Weekly
	case metrics.Monthly:
		return u.Monthly
	}
	return opencode.Window{}
}
