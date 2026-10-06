package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverKeyFromEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("OPENCODE_API_KEY", "oc_sk_env")
	cfg, err := Load("", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "oc_sk_env" {
		t.Errorf("key = %q, want oc_sk_env", cfg.APIKey)
	}
	if cfg.APIKeySource != "$OPENCODE_API_KEY" {
		t.Errorf("source = %q", cfg.APIKeySource)
	}
}

func TestDiscoverKeyFromFile(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("OPENCODE_API_KEY", "")
	t.Setenv("OPENCODE_GO_API_KEY", "")

	dir := filepath.Join(cfgHome, "go-pane")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api_key"), []byte("oc_sk_file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("", 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "oc_sk_file" {
		t.Errorf("key = %q, want oc_sk_file", cfg.APIKey)
	}
}

func TestLoadFlagsOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("OPENCODE_API_KEY", "k")
	cfg, err := Load("127.0.0.1:9999", 30e9, "https://example.test/v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:9999" {
		t.Errorf("addr = %q", cfg.Addr)
	}
	if cfg.BaseURL != "https://example.test/v1" {
		t.Errorf("baseURL = %q", cfg.BaseURL)
	}
}
