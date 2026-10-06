package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscoverKeyFromEnv(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("OPENCODE_API_KEY", "oc_sk_env")
	cfg, err := Load(Options{})
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
	cfg, err := Load(Options{})
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
	cfg, err := Load(Options{Addr: "127.0.0.1:9999", Interval: 30e9, BaseURL: "https://example.test/v1"})
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

// isolate clears the environment that key discovery consults and points HOME
// and the XDG dirs at fresh temp directories.
func isolate(t *testing.T) (configHome, home string) {
	t.Helper()
	home = t.TempDir()
	configHome = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("OPENCODE_API_KEY", "")
	t.Setenv("OPENCODE_GO_API_KEY", "")
	return configHome, home
}

func writeConfig(t *testing.T, configHome, body string) {
	t.Helper()
	dir := filepath.Join(configHome, "go-pane")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFromConfigJSON(t *testing.T) {
	configHome, _ := isolate(t)
	writeConfig(t, configHome, `{"addr":"127.0.0.1:1234","interval":"30s","baseURL":"https://x.test/v1","apiKey":"oc_sk_cfg"}`)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:1234" {
		t.Errorf("addr = %q", cfg.Addr)
	}
	if cfg.Interval != 30*time.Second {
		t.Errorf("interval = %v", cfg.Interval)
	}
	if cfg.BaseURL != "https://x.test/v1" {
		t.Errorf("baseURL = %q", cfg.BaseURL)
	}
	if cfg.APIKey != "oc_sk_cfg" || cfg.APIKeySource != "config file" {
		t.Errorf("key = %q (%q)", cfg.APIKey, cfg.APIKeySource)
	}
}

func TestLoadOptionsAndEnvOverrideConfig(t *testing.T) {
	configHome, _ := isolate(t)
	writeConfig(t, configHome, `{"addr":"127.0.0.1:1111","apiKey":"oc_sk_cfg"}`)
	t.Setenv("OPENCODE_API_KEY", "oc_sk_env")

	cfg, err := Load(Options{Addr: "127.0.0.1:2222"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:2222" {
		t.Errorf("addr = %q, want the option", cfg.Addr)
	}
	if cfg.APIKey != "oc_sk_env" {
		t.Errorf("key = %q, want env to beat config", cfg.APIKey)
	}
}

func TestLoadAPIKeyFileFromConfig(t *testing.T) {
	configHome, _ := isolate(t)
	keyPath := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyPath, []byte("oc_sk_fromfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, configHome, `{"apiKeyFile":`+fmt.Sprintf("%q", keyPath)+`}`)

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "oc_sk_fromfile" || cfg.APIKeySource != keyPath {
		t.Errorf("key = %q (%q)", cfg.APIKey, cfg.APIKeySource)
	}
}

func TestLoadMalformedConfigAndIntervalFallBack(t *testing.T) {
	configHome, _ := isolate(t)
	writeConfig(t, configHome, `{not json`)
	t.Setenv("OPENCODE_API_KEY", "oc_sk_env")

	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interval != DefaultInterval || cfg.Addr != DefaultAddr {
		t.Errorf("defaults not used: %+v", cfg)
	}
	if cfg.APIKey != "oc_sk_env" {
		t.Errorf("key = %q", cfg.APIKey)
	}
}

func TestLoadInvalidIntervalIgnored(t *testing.T) {
	configHome, _ := isolate(t)
	writeConfig(t, configHome, `{"interval":"not-a-duration","apiKey":"k"}`)
	cfg, err := Load(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Interval != DefaultInterval {
		t.Errorf("interval = %v, want default", cfg.Interval)
	}
}

func TestLoadNoKey(t *testing.T) {
	isolate(t)
	if _, err := Load(Options{}); err == nil {
		t.Error("want an error when no key can be found")
	}
}

func TestReadKeyFileEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readKeyFile(path, io.Discard); err == nil {
		t.Error("want an error for an empty key file")
	}
}

func TestReadKeyFileWarnsOnPermissiveMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, []byte("oc_sk_x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	var warn bytes.Buffer
	key, err := readKeyFile(path, &warn)
	if err != nil {
		t.Fatal(err)
	}
	if key != "oc_sk_x" {
		t.Errorf("key = %q", key)
	}
	if !strings.Contains(warn.String(), "chmod 600") {
		t.Errorf("warning = %q, want a chmod hint", warn.String())
	}
}

func TestReadPiAuth(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{"opencode-go", `{"opencode-go":{"type":"api_key","key":"k1"}}`, "k1", true},
		{"opencode fallback", `{"opencode":{"key":"k2"}}`, "k2", true},
		{"missing", `{}`, "", false},
		{"invalid", `not json`, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "auth.json")
			if err := os.WriteFile(path, []byte(c.body), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := readPiAuth(path)
			if c.ok && (err != nil || got != c.want) {
				t.Fatalf("readPiAuth = %q, %v; want %q", got, err, c.want)
			}
			if !c.ok && err == nil {
				t.Fatalf("readPiAuth = %q, want an error", got)
			}
		})
	}
}

func TestResolveKeyPrecedence(t *testing.T) {
	t.Run("flag beats env", func(t *testing.T) {
		t.Setenv("OPENCODE_API_KEY", "env")
		flagFile := filepath.Join(t.TempDir(), "key")
		if err := os.WriteFile(flagFile, []byte("flagkey"), 0o600); err != nil {
			t.Fatal(err)
		}
		key, source, err := resolveKey(flagFile, fileConfig{APIKey: "cfg"}, io.Discard)
		if err != nil || key != "flagkey" || source != flagFile {
			t.Fatalf("got %q (%q), %v", key, source, err)
		}
	})

	t.Run("env beats config", func(t *testing.T) {
		t.Setenv("OPENCODE_API_KEY", "envkey")
		key, source, err := resolveKey("", fileConfig{APIKey: "cfg"}, io.Discard)
		if err != nil || key != "envkey" || source != "$OPENCODE_API_KEY" {
			t.Fatalf("got %q (%q), %v", key, source, err)
		}
	})

	t.Run("config apiKey beats file", func(t *testing.T) {
		configHome, _ := isolate(t)
		dir := filepath.Join(configHome, "go-pane")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "api_key"), []byte("dedicated"), 0o600); err != nil {
			t.Fatal(err)
		}
		key, source, err := resolveKey("", fileConfig{APIKey: "cfgkey"}, io.Discard)
		if err != nil || key != "cfgkey" || source != "config file" {
			t.Fatalf("got %q (%q), %v", key, source, err)
		}
	})

	t.Run("config apiKeyFile", func(t *testing.T) {
		isolate(t)
		keyPath := filepath.Join(t.TempDir(), "key")
		if err := os.WriteFile(keyPath, []byte("fromfile"), 0o600); err != nil {
			t.Fatal(err)
		}
		key, source, err := resolveKey("", fileConfig{APIKeyFile: keyPath}, io.Discard)
		if err != nil || key != "fromfile" || source != keyPath {
			t.Fatalf("got %q (%q), %v", key, source, err)
		}
	})

	t.Run("dedicated api_key file", func(t *testing.T) {
		configHome, _ := isolate(t)
		dir := filepath.Join(configHome, "go-pane")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "api_key")
		if err := os.WriteFile(path, []byte("dedicated"), 0o600); err != nil {
			t.Fatal(err)
		}
		key, source, err := resolveKey("", fileConfig{}, io.Discard)
		if err != nil || key != "dedicated" || source != path {
			t.Fatalf("got %q (%q), %v", key, source, err)
		}
	})

	t.Run("pi auth fallback", func(t *testing.T) {
		_, home := isolate(t)
		dir := filepath.Join(home, ".pi", "agent")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte(`{"opencode-go":{"key":"pikey"}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		key, source, err := resolveKey("", fileConfig{}, io.Discard)
		if err != nil || key != "pikey" || source != "~/.pi/agent/auth.json" {
			t.Fatalf("got %q (%q), %v", key, source, err)
		}
	})

	t.Run("missing", func(t *testing.T) {
		isolate(t)
		if _, _, err := resolveKey("", fileConfig{}, io.Discard); err == nil {
			t.Fatal("want an error")
		}
	})

	t.Run("flag file error is returned", func(t *testing.T) {
		if _, _, err := resolveKey(filepath.Join(t.TempDir(), "nope"), fileConfig{}, io.Discard); err == nil {
			t.Fatal("want an error for a missing flag file")
		}
	})
}

func TestConfigDirAndStateDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("XDG_CONFIG_HOME", "")
	if got, want := ConfigDir(), filepath.Join(home, ".config", "go-pane"); got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
	t.Setenv("XDG_CONFIG_HOME", "/xdg-config")
	if got, want := ConfigDir(), "/xdg-config/go-pane"; got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}

	t.Setenv("XDG_STATE_HOME", "")
	if got, want := StateDir(), filepath.Join(home, ".local", "state", "go-pane"); got != want {
		t.Errorf("StateDir() = %q, want %q", got, want)
	}
	t.Setenv("XDG_STATE_HOME", "/xdg-state")
	if got, want := StateDir(), "/xdg-state/go-pane"; got != want {
		t.Errorf("StateDir() = %q, want %q", got, want)
	}
}
