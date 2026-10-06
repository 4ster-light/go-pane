package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/config"
	"github.com/4ster-light/go-pane/internal/history"
	"github.com/4ster-light/go-pane/internal/opencode"
)

func TestRunBacksOffOnRepeatedFailures(t *testing.T) {
	f := &fakeFetcher{err: errors.New("boom")}
	d := newTestDaemon(t, f)
	d.cfg.Interval = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	if f.calls < 2 {
		t.Errorf("fetch calls = %d, want at least the initial poll plus one retry", f.calls)
	}
	if f.calls > 12 {
		t.Errorf("fetch calls = %d; backoff is not limiting retries", f.calls)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	d := newTestDaemon(t, &fakeFetcher{err: errors.New("boom")})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunKeepsPollingOnSuccess(t *testing.T) {
	reset := time.Now().Add(5 * time.Hour).UTC()
	f := &fakeFetcher{usage: opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 10, ResetsAt: reset},
	}}
	d := newTestDaemon(t, f)
	d.cfg.Interval = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	d.Run(ctx)

	if f.calls < 2 {
		t.Errorf("fetch calls = %d, want repeated polls", f.calls)
	}
	if d.Snapshot().Stale {
		t.Error("successful polls should leave a fresh snapshot")
	}
}

func TestNewAppliesDefaultInterval(t *testing.T) {
	store, err := history.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := New(config.Config{APIKey: "k"}, store, discardLogger())
	if d == nil {
		t.Fatal("New returned nil")
	}
	if d.cfg.Interval != config.DefaultInterval {
		t.Errorf("interval = %v, want %v", d.cfg.Interval, config.DefaultInterval)
	}
}

func TestIntervalClampedWhenNonPositive(t *testing.T) {
	store, err := history.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, interval := range []time.Duration{0, -time.Second} {
		d := newDaemon(config.Config{Interval: interval}, store, discardLogger(), &fakeFetcher{})
		if d.cfg.Interval != config.DefaultInterval {
			t.Errorf("interval %v clamped to %v, want %v", interval, d.cfg.Interval, config.DefaultInterval)
		}
	}
}

func TestPollOnceClearsStaleAfterRecovery(t *testing.T) {
	f := &fakeFetcher{err: errors.New("boom")}
	d := newTestDaemon(t, f)
	if _, err := d.PollOnce(context.Background()); err == nil {
		t.Fatal("first poll should fail")
	}
	if !d.Snapshot().Stale {
		t.Fatal("snapshot should be stale after a failure")
	}

	f.err = nil
	f.usage = opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 42, ResetsAt: time.Now().Add(time.Hour)},
	}
	snap, err := d.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("recovery poll: %v", err)
	}
	if snap.Stale {
		t.Error("stale flag should clear after a successful poll")
	}
	if got := snap.Windows["rolling"].UsedPercent; got != 42 {
		t.Errorf("rolling used = %v, want 42", got)
	}
}
