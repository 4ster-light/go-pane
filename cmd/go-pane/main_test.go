package main

import (
	"bytes"
	"context"
	"encoding/json"
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

// isolateEnv clears the environment key discovery consults and points HOME and
// the XDG dirs at temp directories for in-process command tests.
func isolateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("OPENCODE_API_KEY", "")
	t.Setenv("OPENCODE_GO_API_KEY", "")
}

func TestRunDispatch(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{"version", []string{"version"}, 0, "go-pane ", ""},
		{"--version", []string{"--version"}, 0, "go-pane ", ""},
		{"-v", []string{"-v"}, 0, "go-pane ", ""},
		{"help", []string{"help"}, 0, "Usage:", ""},
		{"--help", []string{"--help"}, 0, "Usage:", ""},
		{"-h", []string{"-h"}, 0, "Usage:", ""},
		{"no args", nil, 2, "", "Usage:"},
		{"unknown", []string{"bogus"}, 2, "", "unknown command"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errBuf bytes.Buffer
			code := run(c.args, &out, &errBuf)
			if code != c.wantCode {
				t.Errorf("exit = %d, want %d", code, c.wantCode)
			}
			if c.wantOut != "" && !strings.Contains(out.String(), c.wantOut) {
				t.Errorf("stdout = %q, want %q", out.String(), c.wantOut)
			}
			if c.wantErr != "" && !strings.Contains(errBuf.String(), c.wantErr) {
				t.Errorf("stderr = %q, want %q", errBuf.String(), c.wantErr)
			}
		})
	}
}

func TestRunOnceInProcess(t *testing.T) {
	isolateEnv(t)
	api := mockAPI(t)
	defer api.Close()
	keyFile := writeKeyFile(t, "oc_sk_test")

	var out, errBuf bytes.Buffer
	if code := run([]string{"once", "--json", "--base-url", api.URL, "--api-key-file", keyFile}, &out, &errBuf); code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errBuf.String())
	}
	var snap metrics.Snapshot
	if err := json.Unmarshal(out.Bytes(), &snap); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out.String())
	}
	if got := snap.Windows["monthly"].UsedPercent; got != 57 {
		t.Errorf("monthly used = %v, want 57", got)
	}

	// `json` is an alias of `once --json`.
	out.Reset()
	errBuf.Reset()
	if code := run([]string{"json", "--base-url", api.URL, "--api-key-file", keyFile}, &out, &errBuf); code != 0 {
		t.Fatalf("json alias exit = %d, stderr = %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "{") {
		t.Errorf("json alias did not print JSON:\n%s", out.String())
	}

	// Plain text mode.
	out.Reset()
	errBuf.Reset()
	if code := run([]string{"once", "--base-url", api.URL, "--api-key-file", keyFile}, &out, &errBuf); code != 0 {
		t.Fatalf("text exit = %d, stderr = %s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Rolling") {
		t.Errorf("text output unexpected:\n%s", out.String())
	}
}

func TestRunDoctorInProcess(t *testing.T) {
	isolateEnv(t)
	api := mockAPI(t)
	defer api.Close()

	var out, errBuf bytes.Buffer
	code := run([]string{"doctor", "--base-url", api.URL, "--api-key-file", writeKeyFile(t, "k")}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %s", code, errBuf.String())
	}
	for _, want := range []string{"API key source:", "Monthly"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunOnceConfigError(t *testing.T) {
	isolateEnv(t)
	var out, errBuf bytes.Buffer
	if code := run([]string{"once"}, &out, &errBuf); code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "configuration error") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}

func TestRunDoctorRequestError(t *testing.T) {
	isolateEnv(t)
	var out, errBuf bytes.Buffer
	code := run([]string{"doctor", "--base-url", "http://127.0.0.1:1", "--api-key-file", writeKeyFile(t, "k")}, &out, &errBuf)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "usage request failed") {
		t.Errorf("stderr = %q", errBuf.String())
	}
}
