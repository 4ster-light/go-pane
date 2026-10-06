// Package app wires the client, history store and metrics into a daemon.
package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/4ster-light/go-pane/internal/config"
	"github.com/4ster-light/go-pane/internal/history"
	"github.com/4ster-light/go-pane/internal/metrics"
	"github.com/4ster-light/go-pane/internal/opencode"
)

// Daemon polls the API and serves the latest computed snapshot.
type Daemon struct {
	cfg    config.Config
	client *opencode.Client
	store  *history.Store
	log    *slog.Logger

	mu   sync.RWMutex
	snap metrics.Snapshot
}

// New builds a daemon.
func New(cfg config.Config, store *history.Store, log *slog.Logger) *Daemon {
	c := opencode.New(cfg.APIKey)
	c.BaseURL = cfg.BaseURL
	if log == nil {
		log = slog.Default()
	}
	return &Daemon{
		cfg:    cfg,
		client: c,
		store:  store,
		log:    log,
		snap: metrics.Snapshot{
			Order:   metrics.Order,
			Windows: map[string]metrics.WindowMetric{},
		},
	}
}

// PollOnce fetches, records and returns a fresh snapshot.
func (d *Daemon) PollOnce(ctx context.Context) (metrics.Snapshot, error) {
	usage, err := d.client.FetchUsage(ctx)
	if err != nil {
		d.mu.Lock()
		s := d.snap
		s.Stale = true
		s.Error = err.Error()
		d.snap = s
		d.mu.Unlock()
		return d.Snapshot(), err
	}
	now := time.Now().UTC()
	d.store.Record(now, usage)
	snap := metrics.Compute(now, usage, d.store.Lengths())
	d.mu.Lock()
	d.snap = snap
	d.mu.Unlock()
	return snap, nil
}

// Snapshot returns the last computed snapshot.
func (d *Daemon) Snapshot() metrics.Snapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.snap
}

// History returns recent samples.
func (d *Daemon) History(limit int) []history.Sample {
	return d.store.Recent(limit)
}

// Run polls until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) {
	if _, err := d.PollOnce(ctx); err != nil {
		d.log.Warn("initial poll failed", "error", err)
	}
	backoff := d.cfg.Interval
	t := time.NewTimer(d.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := d.PollOnce(ctx); err != nil {
				d.log.Warn("poll failed", "error", err)
				backoff *= 2
				if backoff > 10*time.Minute {
					backoff = 10 * time.Minute
				}
				t.Reset(backoff)
				continue
			}
			backoff = d.cfg.Interval
			t.Reset(d.cfg.Interval)
		}
	}
}
