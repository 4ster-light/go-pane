package history

import (
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/metrics"
	"github.com/4ster-light/go-pane/internal/opencode"
)

func TestRecordLearnsWindowLength(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	// First sample establishes the current reset boundary.
	s.Record(base, opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 1, ResetsAt: base.Add(2 * time.Hour)},
		Weekly:  opencode.Window{Status: "ok", Percent: 1, ResetsAt: base.Add(4 * 24 * time.Hour)},
		Monthly: opencode.Window{Status: "ok", Percent: 1, ResetsAt: base.Add(30 * 24 * time.Hour)},
	})
	// Second sample advances the rolling reset by 5h -> learned.
	s.Record(base.Add(time.Hour), opencode.Usage{
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
	s, _ := Open(dir)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		s.Record(base.Add(time.Duration(i)*time.Minute), opencode.Usage{
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
