package metrics

import (
	"bytes"
	"encoding/json"
	"flag"
	"math"
	"os"
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

var update = flag.Bool("update", false, "update golden files")

// TestGoldenSnapshot pins the computed snapshot for the recorded API sample.
// Regenerate with: go test ./internal/metrics -run TestGoldenSnapshot -update
func TestGoldenSnapshot(t *testing.T) {
	body, err := os.ReadFile("../../testdata/usage.sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Usage opencode.Usage `json:"usage"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	now, err := time.Parse(time.RFC3339, "2026-10-06T08:40:48Z")
	if err != nil {
		t.Fatal(err)
	}

	snap := Compute(now, env.Usage, DefaultLengths())
	got, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	const golden = "../../testdata/snapshot.golden.json"
	if *update {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run `go test ./internal/metrics -run TestGoldenSnapshot -update`): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("snapshot differs from %s:\n--- got ---\n%s\n--- want ---\n%s", golden, got, want)
	}
}
