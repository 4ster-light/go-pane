package metrics

import (
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/opencode"
)

func TestFormatPercent(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0%"},
		{0.4, "0.4%"},
		{0.04, "0.0%"},
		{0.999, "1.0%"},
		{1, "1%"},
		{57.4, "57%"},
		{100, "100%"},
	}
	for _, c := range cases {
		if got := FormatPercent(c.in); got != c.want {
			t.Errorf("FormatPercent(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestKindTitle(t *testing.T) {
	cases := map[Kind]string{
		Rolling:       "Rolling",
		Weekly:        "Weekly",
		Monthly:       "Monthly",
		Kind("bogus"): "bogus",
	}
	for k, want := range cases {
		if got := k.Title(); got != want {
			t.Errorf("Kind(%q).Title() = %q, want %q", k, got, want)
		}
	}
}

func TestWindowFor(t *testing.T) {
	u := sampleUsage()
	cases := map[Kind]float64{
		Rolling: 0,
		Weekly:  20,
		Monthly: 57,
	}
	for k, want := range cases {
		if got := WindowFor(k, u).Percent; got != want {
			t.Errorf("WindowFor(%q).Percent = %v, want %v", k, got, want)
		}
	}
	if got := WindowFor(Kind("bogus"), u); got != (opencode.Window{}) {
		t.Errorf("WindowFor(unknown) = %+v, want zero", got)
	}
}

func TestComputeUsesLearnedLengths(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-06T08:40:48Z")
	lengths := Lengths{
		Durations: map[Kind]time.Duration{
			Rolling: 4 * time.Hour,
			Weekly:  6 * 24 * time.Hour,
			Monthly: 28 * 24 * time.Hour,
		},
		Estimated: map[Kind]bool{Monthly: true},
	}
	snap := Compute(now, sampleUsage(), lengths)

	rolling := snap.Windows["rolling"]
	if want := int64((4 * time.Hour) / time.Second); rolling.WindowSeconds != want {
		t.Errorf("rolling windowSeconds = %d, want %d", rolling.WindowSeconds, want)
	}
	if rolling.WindowEstimated {
		t.Error("rolling should not be estimated")
	}
	if !snap.Windows["monthly"].WindowEstimated {
		t.Error("monthly should still be estimated")
	}
}

func TestComputeFallsBackForMissingLengths(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-06T08:40:48Z")
	snap := Compute(now, sampleUsage(), Lengths{Durations: map[Kind]time.Duration{Rolling: 0}})
	if want := int64((5 * time.Hour) / time.Second); snap.Windows["rolling"].WindowSeconds != want {
		t.Errorf("rolling windowSeconds = %d, want default %d", snap.Windows["rolling"].WindowSeconds, want)
	}
}

func TestHeadlineUsesUsedWhenNoProjection(t *testing.T) {
	// At the very start of every window there is no pace, so the headline falls
	// back to the highest usedPercent.
	now := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	u := opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 10, ResetsAt: now.Add(5 * time.Hour)},
		Weekly:  opencode.Window{Status: "ok", Percent: 30, ResetsAt: now.Add(7 * 24 * time.Hour)},
		Monthly: opencode.Window{Status: "ok", Percent: 5, ResetsAt: now.Add(30 * 24 * time.Hour)},
	}
	snap := Compute(now, u, DefaultLengths())
	if snap.Headline != Weekly {
		t.Errorf("headline = %q, want %q", snap.Headline, Weekly)
	}
}

func TestComputeClampsOutOfRange(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	u := opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 150, ResetsAt: now.Add(-time.Hour)},
	}
	snap := Compute(now, u, DefaultLengths())
	m := snap.Windows["rolling"]
	if m.UsedPercent != 100 || m.AvailablePercent != 0 {
		t.Errorf("used/available = %v/%v, want 100/0", m.UsedPercent, m.AvailablePercent)
	}
	if m.RemainingSeconds != 0 {
		t.Errorf("remainingSeconds = %d, want 0 for a past reset", m.RemainingSeconds)
	}
	if m.ElapsedPercent != 100 {
		t.Errorf("elapsedPercent = %v, want 100", m.ElapsedPercent)
	}
}
