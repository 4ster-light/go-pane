// Package metrics turns raw usage into human-readable derived metrics.
package metrics

import (
	"fmt"
	"math"
	"time"

	"github.com/4ster-light/go-pane/internal/opencode"
)

// Kind identifies a usage window.
type Kind string

const (
	Rolling Kind = "rolling"
	Weekly  Kind = "weekly"
	Monthly Kind = "monthly"
)

// Order is the canonical display order.
var Order = []Kind{Rolling, Weekly, Monthly}

// Title returns a human title for the window.
func (k Kind) Title() string {
	switch k {
	case Rolling:
		return "Rolling"
	case Weekly:
		return "Weekly"
	case Monthly:
		return "Monthly"
	}
	return string(k)
}

// DefaultWindowLengths are used until the real cadence is learned from history.
// Rolling ~5h and weekly = 7d are observed; monthly is a signup anniversary and
// varies between 28 and 31 days.
func DefaultWindowLengths() map[Kind]time.Duration {
	return map[Kind]time.Duration{
		Rolling: 5 * time.Hour,
		Weekly:  7 * 24 * time.Hour,
		Monthly: 30 * 24 * time.Hour,
	}
}

// Lengths carries learned window durations plus whether each is still a guess.
type Lengths struct {
	Durations map[Kind]time.Duration
	Estimated map[Kind]bool
}

// DefaultLengths returns all-default (estimated) lengths.
func DefaultLengths() Lengths {
	l := Lengths{Durations: DefaultWindowLengths(), Estimated: map[Kind]bool{}}
	for k := range l.Durations {
		l.Estimated[k] = true
	}
	return l
}

// WindowMetric is the computed view of a single usage window.
type WindowMetric struct {
	Kind                 Kind       `json:"kind"`
	Title                string     `json:"title"`
	Status               string     `json:"status"`
	UsedPercent          float64    `json:"usedPercent"`
	AvailablePercent     float64    `json:"availablePercent"`
	ResetsAt             time.Time  `json:"resetsAt"`
	WindowSeconds        int64      `json:"windowSeconds"`
	WindowEstimated      bool       `json:"windowEstimated"`
	ElapsedPercent       float64    `json:"elapsedPercent"`
	RemainingSeconds     int64      `json:"remainingSeconds"`
	PaceRatio            *float64   `json:"paceRatio"`
	ProjectedUsedPercent *float64   `json:"projectedUsedPercent"`
	ExhaustsAt           *time.Time `json:"exhaustsAt"`
	SafeRatePerHour      float64    `json:"safeRatePerHour"`
	BudgetPerHour        float64    `json:"budgetPerHour"`
}

// Snapshot is the complete computed state served to clients.
type Snapshot struct {
	FetchedAt time.Time               `json:"fetchedAt"`
	Stale     bool                    `json:"stale"`
	Error     string                  `json:"error,omitempty"`
	Headline  Kind                    `json:"headline"`
	Order     []Kind                  `json:"order"`
	Windows   map[string]WindowMetric `json:"windows"`
}

// Compute derives metrics for all windows.
func Compute(now time.Time, u opencode.Usage, lengths Lengths) Snapshot {
	if lengths.Durations == nil {
		lengths = DefaultLengths()
	}
	snap := Snapshot{
		FetchedAt: now.UTC(),
		Order:     Order,
		Windows:   map[string]WindowMetric{},
	}
	defaults := DefaultWindowLengths()
	for _, k := range Order {
		lenDur := lengths.Durations[k]
		if lenDur <= 0 {
			lenDur = defaults[k]
		}
		m := computeWindow(now, k, windowFor(k, u), lenDur)
		m.WindowEstimated = lengths.Estimated[k]
		snap.Windows[string(k)] = m
	}
	snap.Headline = pickHeadline(snap.Windows)
	return snap
}

func windowFor(k Kind, u opencode.Usage) opencode.Window {
	switch k {
	case Rolling:
		return u.Rolling
	case Weekly:
		return u.Weekly
	case Monthly:
		return u.Monthly
	}
	return opencode.Window{}
}

func computeWindow(now time.Time, k Kind, w opencode.Window, windowLen time.Duration) WindowMetric {
	now = now.UTC()
	used := clamp(w.Percent, 0, 100)
	m := WindowMetric{
		Kind:             k,
		Title:            k.Title(),
		Status:           w.Status,
		UsedPercent:      used,
		AvailablePercent: 100 - used,
		ResetsAt:         w.ResetsAt.UTC(),
		WindowSeconds:    int64(windowLen / time.Second),
		BudgetPerHour:    100 / windowLen.Hours(),
	}

	windowStart := w.ResetsAt.Add(-windowLen)
	elapsed := now.Sub(windowStart)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > windowLen {
		elapsed = windowLen
	}
	elapsedFrac := 0.0
	if windowLen > 0 {
		elapsedFrac = elapsed.Seconds() / windowLen.Seconds()
	}
	m.ElapsedPercent = clamp(elapsedFrac*100, 0, 100)

	remaining := w.ResetsAt.Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	m.RemainingSeconds = int64(remaining / time.Second)
	if remaining > 0 {
		m.SafeRatePerHour = (100 - used) / remaining.Hours()
	}

	usedFrac := used / 100
	if elapsedFrac > 0 {
		pace := usedFrac / elapsedFrac
		m.PaceRatio = &pace
		proj := pace * 100
		m.ProjectedUsedPercent = &proj
		if usedFrac > 0 {
			ttx := time.Duration(float64(elapsed) * (1 - usedFrac) / usedFrac)
			at := now.Add(ttx)
			m.ExhaustsAt = &at
		}
	}
	return m
}

func pickHeadline(ws map[string]WindowMetric) Kind {
	best := Kind("")
	bestScore := math.Inf(-1)
	for _, k := range Order {
		m, ok := ws[string(k)]
		if !ok {
			continue
		}
		score := m.UsedPercent
		if m.ProjectedUsedPercent != nil {
			score = *m.ProjectedUsedPercent
		}
		if score > bestScore {
			bestScore = score
			best = k
		}
	}
	return best
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}

// FormatDuration renders a positive duration as "3d 4h", "4h 59m" or "12m".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Minute)
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	mins := d / time.Minute
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, mins)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

// FormatPercent renders a percentage, keeping one decimal below 1%.
func FormatPercent(v float64) string {
	if v > 0 && v < 1 {
		return fmt.Sprintf("%.1f%%", v)
	}
	return fmt.Sprintf("%.0f%%", v)
}
