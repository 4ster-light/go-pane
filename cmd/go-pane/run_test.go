package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/4ster-light/go-pane/internal/metrics"
)

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
