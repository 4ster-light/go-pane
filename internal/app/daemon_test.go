package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/config"
	"github.com/4ster-light/go-pane/internal/history"
	"github.com/4ster-light/go-pane/internal/opencode"
)

type fakeFetcher struct {
	usage opencode.Usage
	err   error
	calls int
}

func (f *fakeFetcher) FetchUsage(ctx context.Context) (opencode.Usage, error) {
	f.calls++
	return f.usage, f.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestDaemon(t *testing.T, f Fetcher) *Daemon {
	t.Helper()
	store, err := history.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return newDaemon(config.Config{}, store, discardLogger(), f)
}

func TestPollOnceRecordsHistory(t *testing.T) {
	reset := time.Now().Add(5 * time.Hour).UTC()
	f := &fakeFetcher{usage: opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 10, ResetsAt: reset},
	}}
	d := newTestDaemon(t, f)

	snap, err := d.PollOnce(context.Background())
	if err != nil {
		t.Fatalf("PollOnce: %v", err)
	}
	if got := snap.Windows["rolling"].UsedPercent; got != 10 {
		t.Errorf("rolling used = %v, want 10", got)
	}
	if snap.Stale {
		t.Error("fresh snapshot should not be stale")
	}
	if got := len(d.History(10)); got != 1 {
		t.Errorf("history samples = %d, want 1", got)
	}
	if f.calls != 1 {
		t.Errorf("fetch calls = %d, want 1", f.calls)
	}
}

func TestPollOnceErrorMarksStale(t *testing.T) {
	d := newTestDaemon(t, &fakeFetcher{err: errors.New("boom")})

	if _, err := d.PollOnce(context.Background()); err == nil {
		t.Fatal("PollOnce: want error, got nil")
	}
	snap := d.Snapshot()
	if !snap.Stale {
		t.Error("snapshot should be stale after a failed poll")
	}
	if snap.Error == "" {
		t.Error("snapshot should carry the error message")
	}
}
