// Package history persists usage samples and learns real window lengths.
package history

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/4ster-light/go-pane/internal/metrics"
	"github.com/4ster-light/go-pane/internal/opencode"
)

// maxLineBytes bounds a single JSONL record so a corrupt file cannot exhaust
// memory while scanning.
const maxLineBytes = 1 << 20

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
	if err := json.Unmarshal(b, &lf); err != nil {
		fmt.Fprintf(os.Stderr, "warning: ignoring malformed %s: %v\n", s.lengthsPath(), err)
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

func (s *Store) saveLengthsLocked() error {
	out := learnedFile{Durations: map[string]int64{}}
	for k, d := range s.lengths.Durations {
		if s.lengths.Estimated[k] {
			continue
		}
		out.Durations[string(k)] = int64(d / time.Second)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	return writeFileAtomic(s.lengthsPath(), b, 0o644)
}

func (s *Store) loadLastSample() {
	lines, err := readLines(s.historyPath())
	if err != nil {
		return
	}
	// Walk backwards to the most recent decodable sample.
	for i := len(lines) - 1; i >= 0; i-- {
		var smp Sample
		if json.Unmarshal(lines[i], &smp) != nil {
			continue
		}
		for k, t := range smp.ResetsAt {
			s.last[metrics.Kind(k)] = t
		}
		return
	}
}

// Record appends a sample and learns window lengths when resets advance.
func (s *Store) Record(now time.Time, u opencode.Usage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	smp := Sample{
		Time:     now.UTC(),
		Percents: map[string]float64{},
		ResetsAt: map[string]time.Time{},
	}
	learnedChanged := false
	for _, k := range metrics.Order {
		w := metrics.WindowFor(k, u)
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
		// A failure to persist learned lengths must not drop the sample.
		_ = s.saveLengthsLocked()
	}

	enc, err := json.Marshal(smp)
	if err != nil {
		return err
	}
	enc = append(enc, '\n')

	f, err := os.OpenFile(s.historyPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err = f.Write(enc); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
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
	lines, err := readLines(s.historyPath())
	if err != nil {
		return nil
	}
	all := make([]Sample, 0, len(lines))
	for _, line := range lines {
		var smp Sample
		if json.Unmarshal(line, &smp) == nil {
			all = append(all, smp)
		}
	}
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all
}

// trim bounds the history file to a fixed number of records once it grows past
// a threshold.
func (s *Store) trim() {
	const (
		trimThreshold = 8 << 20
		keepRecords   = 10000
	)
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = trimFile(s.historyPath(), trimThreshold, keepRecords)
}

// trimFile rewrites path with its last keep lines when the file is at least
// threshold bytes. It is a no-op if the file is missing, smaller than the
// threshold, or already within keep lines.
func trimFile(path string, threshold int64, keep int) error {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() < threshold {
		return nil
	}
	lines, err := readLines(path)
	if err != nil {
		return err
	}
	if len(lines) <= keep {
		return nil
	}
	lines = lines[len(lines)-keep:]

	var buf []byte
	for _, l := range lines {
		buf = append(buf, l...)
		buf = append(buf, '\n')
	}
	return writeFileAtomic(path, buf, 0o644)
}

// readLines returns the lines of path. A missing file yields no lines and no
// error.
func readLines(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)
	var lines [][]byte
	for sc.Scan() {
		lines = append(lines, append([]byte(nil), sc.Bytes()...))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// writeFileAtomic writes data to path via a temporary file and rename.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// plausibleWindow guards against learning a bogus length when the daemon was
// offline across one or more resets. The rolling bound is deliberately tight:
// a missed 5h reset would otherwise teach a 10h window.
func plausibleWindow(k metrics.Kind, d time.Duration) bool {
	switch k {
	case metrics.Rolling:
		return d >= 4*time.Hour && d <= 6*time.Hour
	case metrics.Weekly:
		return d >= 6*24*time.Hour && d <= 8*24*time.Hour
	case metrics.Monthly:
		return d >= 25*24*time.Hour && d <= 35*24*time.Hour
	}
	return false
}
