// Package config resolves runtime settings and the OpenCode API key.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/4ster-light/go-pane/internal/opencode"
)

const (
	DefaultAddr     = "127.0.0.1:17873"
	DefaultInterval = 60 * time.Second
)

// Config is the resolved runtime configuration.
type Config struct {
	Addr         string
	Interval     time.Duration
	BaseURL      string
	APIKey       string
	APIKeySource string
	StateDir     string
	ConfigDir    string
}

type fileConfig struct {
	Addr       string `json:"addr"`
	Interval   string `json:"interval"`
	BaseURL    string `json:"baseURL"`
	APIKey     string `json:"apiKey"`
	APIKeyFile string `json:"apiKeyFile"`
}

// ConfigDir returns the per-user config directory.
func ConfigDir() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return filepath.Join(v, "go-pane")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "go-pane")
}

// StateDir returns the per-user state directory.
func StateDir() string {
	if v := os.Getenv("XDG_STATE_HOME"); v != "" {
		return filepath.Join(v, "go-pane")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".local", "state", "go-pane")
}

// Load builds the configuration. Explicit flags win over file/env/defaults.
func Load(addr string, interval time.Duration, baseURL, keyFile string) (Config, error) {
	cfg := Config{
		Addr:      DefaultAddr,
		Interval:  DefaultInterval,
		BaseURL:   opencode.DefaultBaseURL,
		StateDir:  StateDir(),
		ConfigDir: ConfigDir(),
	}

	var fc fileConfig
	if b, err := os.ReadFile(filepath.Join(cfg.ConfigDir, "config.json")); err == nil {
		_ = json.Unmarshal(b, &fc)
	}
	if fc.Addr != "" {
		cfg.Addr = fc.Addr
	}
	if fc.Interval != "" {
		if d, err := time.ParseDuration(fc.Interval); err == nil && d > 0 {
			cfg.Interval = d
		}
	}
	if fc.BaseURL != "" {
		cfg.BaseURL = fc.BaseURL
	}
	if fc.APIKey != "" {
		cfg.APIKey = fc.APIKey
		cfg.APIKeySource = "config file"
	}
	if addr != "" {
		cfg.Addr = addr
	}
	if interval > 0 {
		cfg.Interval = interval
	}
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}

	if keyFile == "" {
		keyFile = fc.APIKeyFile
	}
	if keyFile != "" {
		key, err := readKeyFile(keyFile)
		if err != nil {
			return cfg, err
		}
		cfg.APIKey = key
		cfg.APIKeySource = keyFile
		return cfg, nil
	}

	if cfg.APIKey == "" {
		key, source, err := discoverKey()
		if err != nil {
			return cfg, err
		}
		cfg.APIKey = key
		cfg.APIKeySource = source
	}
	return cfg, nil
}

func discoverKey() (string, string, error) {
	for _, env := range []string{"OPENCODE_API_KEY", "OPENCODE_GO_API_KEY"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, "$" + env, nil
		}
	}
	if p := filepath.Join(ConfigDir(), "api_key"); fileExists(p) {
		if key, err := readKeyFile(p); err == nil {
			return key, p, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if key, err := readPiAuth(filepath.Join(home, ".pi", "agent", "auth.json")); err == nil {
			return key, "~/.pi/agent/auth.json", nil
		}
	}
	return "", "", errors.New("no OpenCode API key found; set OPENCODE_API_KEY, create ~/.config/go-pane/api_key (chmod 600), or pass --api-key-file")
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func readKeyFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read key file %s: %w", path, err)
	}
	key := strings.TrimSpace(string(b))
	if key == "" {
		return "", fmt.Errorf("key file %s is empty", path)
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().Perm()&0o077 != 0 {
		fmt.Fprintf(os.Stderr, "warning: %s is readable by others (%o); run: chmod 600 %s\n", path, fi.Mode().Perm(), path)
	}
	return key, nil
}

func readPiAuth(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var data map[string]struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return "", err
	}
	if v, ok := data["opencode-go"]; ok && v.Key != "" {
		return v.Key, nil
	}
	if v, ok := data["opencode"]; ok && v.Key != "" {
		return v.Key, nil
	}
	return "", errors.New("no opencode-go key in pi auth store")
}
