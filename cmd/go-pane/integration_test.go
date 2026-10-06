package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/4ster-light/go-pane/internal/metrics"
)

var (
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

// cliBinary builds the go-pane command once and returns its path.
func cliBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "go-pane-cli")
		if err != nil {
			buildErr = err
			return
		}
		binPath = filepath.Join(dir, "go-pane")
		cmd := exec.CommandContext(context.Background(), "go", "build", "-o", binPath, ".")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("go build: %w\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binPath
}

// mockAPI serves the recorded usage payload for any path.
func mockAPI(t *testing.T) *httptest.Server {
	t.Helper()
	body := sampleBody(t)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
}

func writeKeyFile(t *testing.T, key string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "api_key")
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// isolatedEnv points the CLI at throwaway XDG directories so tests never touch
// the developer's real history or config.
func isolatedEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(),
		"XDG_STATE_HOME="+t.TempDir(),
		"XDG_CONFIG_HOME="+t.TempDir(),
	)
}

func freeAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String()
}

func TestCLIServeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := cliBinary(t)
	api := mockAPI(t)
	defer api.Close()
	keyFile := writeKeyFile(t, "oc_sk_test")
	addr := freeAddr(t)

	cmd := exec.CommandContext(context.Background(), bin, "serve",
		"--addr", addr,
		"--interval", "50ms",
		"--base-url", api.URL,
		"--api-key-file", keyFile,
	)
	cmd.Env = isolatedEnv(t)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()

	base := "http://" + addr
	ready := false
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/healthz") //nolint:noctx // test polling
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("daemon never became ready; stderr:\n%s", stderr.String())
	}

	resp, err := http.Get(base + "/v1/usage") //nolint:noctx // test request
	if err != nil {
		t.Fatalf("GET /v1/usage: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var snap metrics.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if got := snap.Windows["monthly"].UsedPercent; got != 57 {
		t.Errorf("monthly used = %v, want 57", got)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	select {
	case err := <-waitCh:
		if err != nil {
			t.Errorf("serve exited with %v; stderr:\n%s", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-waitCh
		t.Errorf("serve did not stop after SIGTERM; stderr:\n%s", stderr.String())
	}
}

func TestCLIOnceJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := cliBinary(t)
	api := mockAPI(t)
	defer api.Close()

	cmd := exec.CommandContext(context.Background(), bin, "once", "--json",
		"--base-url", api.URL,
		"--api-key-file", writeKeyFile(t, "oc_sk_test"),
	)
	cmd.Env = isolatedEnv(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("once --json: %v", err)
	}
	var snap metrics.Snapshot
	if err := json.Unmarshal(out, &snap); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if got := snap.Windows["weekly"].UsedPercent; got != 20 {
		t.Errorf("weekly used = %v, want 20", got)
	}
}

func TestCLIDoctor(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	bin := cliBinary(t)
	api := mockAPI(t)
	defer api.Close()

	cmd := exec.CommandContext(context.Background(), bin, "doctor",
		"--base-url", api.URL,
		"--api-key-file", writeKeyFile(t, "oc_sk_test"),
	)
	cmd.Env = isolatedEnv(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !bytes.Contains(out, []byte("API key source:")) || !bytes.Contains(out, []byte("Monthly")) {
		t.Errorf("doctor output unexpected:\n%s", out)
	}
}

func TestCLIVersion(t *testing.T) {
	bin := cliBinary(t)
	out, err := exec.CommandContext(context.Background(), bin, "version").Output()
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if !bytes.HasPrefix(out, []byte("go-pane ")) {
		t.Errorf("version output = %q", out)
	}
}

func TestCLIUsageAndUnknownCommand(t *testing.T) {
	bin := cliBinary(t)

	out, err := exec.CommandContext(context.Background(), bin).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("no args: err = %v, want exit 2", err)
	}
	if !bytes.Contains(out, []byte("Usage:")) {
		t.Errorf("usage text missing:\n%s", out)
	}

	out, err = exec.CommandContext(context.Background(), bin, "bogus").CombinedOutput()
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("unknown command: err = %v, want exit 2", err)
	}
	if !bytes.Contains(out, []byte("unknown command")) {
		t.Errorf("unknown command text missing:\n%s", out)
	}
}
