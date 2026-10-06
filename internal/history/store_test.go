package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/metrics"
	"github.com/4ster-light/go-pane/internal/opencode"
)

func mustRecord(t *testing.T, s *Store, now time.Time, u opencode.Usage) {
	t.Helper()
	if err := s.Record(now, u); err != nil {
		t.Fatalf("Record: %v", err)
	}
}

func TestRecordLearnsWindowLength(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	// First sample establishes the current reset boundary.
	mustRecord(t, s, base, opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 1, ResetsAt: base.Add(2 * time.Hour)},
		Weekly:  opencode.Window{Status: "ok", Percent: 1, ResetsAt: base.Add(4 * 24 * time.Hour)},
		Monthly: opencode.Window{Status: "ok", Percent: 1, ResetsAt: base.Add(30 * 24 * time.Hour)},
	})
	// Second sample advances the resets -> lengths are learned.
	mustRecord(t, s, base.Add(time.Hour), opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 2, ResetsAt: base.Add(2*time.Hour + 5*time.Hour)},
		Weekly:  opencode.Window{Status: "ok", Percent: 2, ResetsAt: base.Add(4*24*time.Hour + 7*24*time.Hour)},
		Monthly: opencode.Window{Status: "ok", Percent: 2, ResetsAt: base.Add(30*24*time.Hour + 31*24*time.Hour)},
	})

	l := s.Lengths()
	if l.Estimated[metrics.Rolling] {
		t.Errorf("rolling should no longer be estimated")
	}
	if got := l.Durations[metrics.Rolling]; got != 5*time.Hour {
		t.Errorf("rolling length = %v, want 5h", got)
	}
	if got := l.Durations[metrics.Weekly]; got != 7*24*time.Hour {
		t.Errorf("weekly length = %v, want 7d", got)
	}
	if got := l.Durations[metrics.Monthly]; got != 31*24*time.Hour {
		t.Errorf("monthly length = %v, want 31d", got)
	}
}

func TestPlausibleWindowRejectsBogus(t *testing.T) {
	if plausibleWindow(metrics.Rolling, 40*time.Hour) {
		t.Errorf("40h should not be a plausible rolling window")
	}
	if !plausibleWindow(metrics.Weekly, 7*24*time.Hour) {
		t.Errorf("7d should be a plausible weekly window")
	}
}

func TestRecent(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		mustRecord(t, s, base.Add(time.Duration(i)*time.Minute), opencode.Usage{
			Rolling: opencode.Window{Status: "ok", Percent: float64(i), ResetsAt: base.Add(time.Hour)},
		})
	}
	got := s.Recent(3)
	if len(got) != 3 {
		t.Fatalf("Recent(3) = %d samples, want 3", len(got))
	}
	if got[2].Percents["rolling"] != 4 {
		t.Errorf("last sample rolling = %v, want 4", got[2].Percents["rolling"])
	}
}

func TestRecentSkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	mustRecord(t, s, time.Now(), opencode.Usage{})

	f, err := os.OpenFile(filepath.Join(dir, "history.jsonl"), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json}\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if got := len(s.Recent(0)); got != 1 {
		t.Errorf("Recent = %d samples, want 1 (corrupt line skipped)", got)
	}
}
