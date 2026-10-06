// Package config resolves runtime settings and the OpenCode API key.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Options carries the command-line overrides. Zero values fall back to the
// config file and then to the built-in defaults.
type Options struct {
	Addr     string
	Interval time.Duration
	BaseURL  string
	KeyFile  string
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

// Load builds the configuration. Explicit options win over the config file,
// which wins over the defaults. The API key is resolved in this order:
//
//  1. --api-key-file
//  2. OPENCODE_API_KEY / OPENCODE_GO_API_KEY
//  3. config.json apiKey / apiKeyFile
//  4. ~/.config/go-pane/api_key
//  5. ~/.pi/agent/auth.json opencode-go.key
func Load(opts Options) (Config, error) {
	cfg := Config{
		Addr:      DefaultAddr,
		Interval:  DefaultInterval,
		BaseURL:   opencode.DefaultBaseURL,
		StateDir:  StateDir(),
		ConfigDir: ConfigDir(),
	}

	var fc fileConfig
	configPath := filepath.Join(cfg.ConfigDir, "config.json")
	if b, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(b, &fc); err != nil {
			fmt.Fprintf(os.Stderr, "warning: ignoring malformed %s: %v\n", configPath, err)
		}
	}
	if fc.Addr != "" {
		cfg.Addr = fc.Addr
	}
	if fc.Interval != "" {
		if d, err := time.ParseDuration(fc.Interval); err == nil && d > 0 {
			cfg.Interval = d
		} else {
			fmt.Fprintf(os.Stderr, "warning: ignoring invalid interval %q in %s\n", fc.Interval, configPath)
		}
	}
	if fc.BaseURL != "" {
		cfg.BaseURL = fc.BaseURL
	}
	if opts.Addr != "" {
		cfg.Addr = opts.Addr
	}
	if opts.Interval > 0 {
		cfg.Interval = opts.Interval
	}
	if opts.BaseURL != "" {
		cfg.BaseURL = opts.BaseURL
	}

	key, source, err := resolveKey(opts.KeyFile, fc, os.Stderr)
	if err != nil {
		return cfg, err
	}
	cfg.APIKey = key
	cfg.APIKeySource = source
	return cfg, nil
}

// resolveKey implements the documented key-discovery precedence. The warn
// writer receives permission warnings; nil means os.Stderr.
func resolveKey(flagKeyFile string, fc fileConfig, warn io.Writer) (string, string, error) {
	if flagKeyFile != "" {
		key, err := readKeyFile(flagKeyFile, warn)
		if err != nil {
			return "", "", err
		}
		return key, flagKeyFile, nil
	}
	for _, env := range []string{"OPENCODE_API_KEY", "OPENCODE_GO_API_KEY"} {
		if v := strings.TrimSpace(os.Getenv(env)); v != "" {
			return v, "$" + env, nil
		}
	}
	if fc.APIKey != "" {
		return fc.APIKey, "config file", nil
	}
	if fc.APIKeyFile != "" {
		key, err := readKeyFile(fc.APIKeyFile, warn)
		if err != nil {
			return "", "", err
		}
		return key, fc.APIKeyFile, nil
	}
	if p := filepath.Join(ConfigDir(), "api_key"); fileExists(p) {
		if key, err := readKeyFile(p, warn); err == nil {
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

func readKeyFile(path string, warn io.Writer) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read key file %s: %w", path, err)
	}
	key := strings.TrimSpace(string(b))
	if key == "" {
		return "", fmt.Errorf("key file %s is empty", path)
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().Perm()&0o077 != 0 {
		if warn == nil {
			warn = os.Stderr
		}
		_, _ = fmt.Fprintf(warn, "warning: %s is readable by others (%o); run: chmod 600 %s\n", path, fi.Mode().Perm(), path)
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
