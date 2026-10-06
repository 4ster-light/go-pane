package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/metrics"
	"github.com/4ster-light/go-pane/internal/opencode"
)

func sampleBody(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("../../testdata/usage.sample.json")
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestFetchWithRetrySucceedsAfterFailure(t *testing.T) {
	body := sampleBody(t)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := opencode.New("k")
	c.BaseURL = srv.URL
	u, err := fetchWithRetry(context.Background(), c, 3)
	if err != nil {
		t.Fatalf("fetchWithRetry: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	if u.Monthly.Percent != 57 {
		t.Errorf("monthly percent = %v, want 57", u.Monthly.Percent)
	}
}

func TestFetchWithRetryStopsOnAuth(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := opencode.New("k")
	c.BaseURL = srv.URL
	if _, err := fetchWithRetry(context.Background(), c, 3); !opencode.IsKind(err, opencode.KindAuth) {
		t.Fatalf("err = %v, want auth", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (auth must not be retried)", calls)
	}
}

func TestFetchWithRetryContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := opencode.New("k")
	c.BaseURL = "http://127.0.0.1:1"
	if _, err := fetchWithRetry(ctx, c, 3); err == nil {
		t.Error("want an error for a canceled context")
	}
}

func TestPrintText(t *testing.T) {
	now := time.Date(2026, 10, 6, 8, 40, 48, 0, time.UTC)
	u := opencode.Usage{
		Rolling: opencode.Window{Status: "ok", Percent: 0, ResetsAt: now.Add(5 * time.Hour)},
		Weekly:  opencode.Window{Status: "ok", Percent: 20, ResetsAt: now.Add(6 * 24 * time.Hour)},
	}
	snap := metrics.Compute(now, u, metrics.DefaultLengths())

	var buf bytes.Buffer
	printText(&buf, snap)
	out := buf.String()
	for _, want := range []string{"Rolling", "Weekly", "% left", "pace", "empty in"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	buf.Reset()
	snap.Stale = true
	snap.Error = "daemon down"
	printText(&buf, snap)
	if !strings.Contains(buf.String(), "stale: daemon down") {
		t.Errorf("stale error not printed:\n%s", buf.String())
	}
}

func TestNewLoggerLevels(t *testing.T) {
	ctx := context.Background()
	if !newLogger("debug").Enabled(ctx, slog.LevelDebug) {
		t.Error("debug logger should enable debug")
	}
	if newLogger("error").Enabled(ctx, slog.LevelInfo) {
		t.Error("error logger should suppress info")
	}
	for _, lvl := range []string{"info", "warn", "warning", "bogus", ""} {
		if newLogger(lvl) == nil {
			t.Errorf("newLogger(%q) returned nil", lvl)
		}
	}
}
