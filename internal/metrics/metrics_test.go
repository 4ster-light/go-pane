package metrics

import (
	"math"
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/opencode"
)

func sampleUsage() opencode.Usage {
	must := func(s string) time.Time {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			panic(err)
		}
		return t
	}
	return opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 0, ResetsAt: must("2026-10-06T13:39:32Z")},
		Weekly:  opencode.Window{Status: "ok", Percent: 20, ResetsAt: must("2026-10-12T00:00:00Z")},
		Monthly: opencode.Window{Status: "ok", Percent: 57, ResetsAt: must("2026-10-25T14:02:02Z")},
	}
}

func approx(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %v, want %v (±%v)", name, got, want, tol)
	}
}

func TestCompute(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-06T08:40:48Z")
	snap := Compute(now, sampleUsage(), DefaultLengths())

	if got := len(snap.Windows); got != 3 {
		t.Fatalf("windows = %d, want 3", got)
	}

	rolling := snap.Windows["rolling"]
	approx(t, "rolling.used", rolling.UsedPercent, 0, 0.001)
	approx(t, "rolling.available", rolling.AvailablePercent, 100, 0.001)
	if rolling.ExhaustsAt != nil {
		t.Errorf("rolling should have no exhaustion ETA when unused")
	}
	approx(t, "rolling.remainingSeconds", float64(rolling.RemainingSeconds), 4*3600+58*60+44, 2)

	weekly := snap.Windows["weekly"]
	approx(t, "weekly.pace", *weekly.PaceRatio, 1.0281, 0.01)
	approx(t, "weekly.projected", *weekly.ProjectedUsedPercent, 102.81, 0.5)
	if weekly.ExhaustsAt == nil || !weekly.ExhaustsAt.Before(weekly.ResetsAt) {
		t.Errorf("weekly should exhaust before reset")
	}

	monthly := snap.Windows["monthly"]
	approx(t, "monthly.pace", *monthly.PaceRatio, 1.5867, 0.01)
	approx(t, "monthly.projected", *monthly.ProjectedUsedPercent, 158.67, 0.5)
	if monthly.SafeRatePerHour <= 0 {
		t.Errorf("monthly.safeRatePerHour = %v, want > 0", monthly.SafeRatePerHour)
	}
}

func TestComputeNoElapsed(t *testing.T) {
	// Right at the window boundary: pace must be nil, not NaN/Inf.
	reset := time.Date(2026, 10, 6, 13, 39, 32, 0, time.UTC)
	now := reset.Add(-5 * time.Hour)
	u := opencode.Usage{Rolling: opencode.Window{Status: "ok", Percent: 0, ResetsAt: reset}}
	snap := Compute(now, u, DefaultLengths())
	if snap.Windows["rolling"].PaceRatio != nil {
		t.Errorf("pace should be nil at window start, got %v", *snap.Windows["rolling"].PaceRatio)
	}
}

func TestHeadlinePicksMostConstrained(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-10-06T08:40:48Z")
	snap := Compute(now, sampleUsage(), DefaultLengths())
	if snap.Headline != Monthly {
		t.Errorf("headline = %q, want %q", snap.Headline, Monthly)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{19*24*time.Hour + 5*time.Hour + 21*time.Minute, "19d 5h"},
		{4*time.Hour + 58*time.Minute, "4h 58m"},
		{12 * time.Minute, "12m"},
		{-time.Minute, "0m"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.d); got != c.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}
