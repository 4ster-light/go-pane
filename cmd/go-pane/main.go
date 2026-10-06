// Command go-pane polls OpenCode Go usage and serves it to the KDE plasmoid.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/4ster-light/go-pane/internal/app"
	"github.com/4ster-light/go-pane/internal/config"
	"github.com/4ster-light/go-pane/internal/history"
	"github.com/4ster-light/go-pane/internal/metrics"
	"github.com/4ster-light/go-pane/internal/opencode"
	"github.com/4ster-light/go-pane/internal/server"
	"github.com/4ster-light/go-pane/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		os.Exit(runServe(os.Args[2:]))
	case "once", "json":
		os.Exit(runOnce(os.Args[1] == "json", os.Args[2:]))
	case "doctor":
		os.Exit(runDoctor(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Printf("go-pane %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `go-pane — OpenCode Go usage for the KDE Plasma widget

Usage:
  go-pane serve   [--addr 127.0.0.1:17873] [--interval 60s] [--base-url URL] [--api-key-file PATH] [--log-level info]
  go-pane once    [--json] [--base-url URL] [--api-key-file PATH]
  go-pane json    [--base-url URL] [--api-key-file PATH]
  go-pane doctor  [--base-url URL] [--api-key-file PATH]
  go-pane version
`)
}

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "", "listen address (default 127.0.0.1:17873)")
	interval := fs.Duration("interval", 0, "poll interval (default 60s)")
	baseURL := fs.String("base-url", "", "API base URL")
	keyFile := fs.String("api-key-file", "", "file containing the API key")
	logLevel := fs.String("log-level", "info", "log level: debug, info, warn, error")
	_ = fs.Parse(args)

	log := newLogger(*logLevel)
	cfg, err := config.Load(config.Options{Addr: *addr, Interval: *interval, BaseURL: *baseURL, KeyFile: *keyFile})
	if err != nil {
		log.Error("configuration error", "error", err)
		return 1
	}
	store, err := history.Open(cfg.StateDir)
	if err != nil {
		log.Error("open history store", "error", err)
		return 1
	}
	d := app.New(cfg, store, log)
	srv := server.New(cfg.Addr, d)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go d.Run(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("go-pane serving",
		"addr", cfg.Addr,
		"interval", cfg.Interval.String(),
		"base", cfg.BaseURL,
		"key", cfg.APIKeySource)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("http server", "error", err)
		return 1
	}
	return 0
}

func runOnce(asJSON bool, args []string) int {
	fs := flag.NewFlagSet("once", flag.ExitOnError)
	jsonFlag := fs.Bool("json", asJSON, "print JSON instead of text")
	baseURL := fs.String("base-url", "", "API base URL")
	keyFile := fs.String("api-key-file", "", "file containing the API key")
	_ = fs.Parse(args)

	cfg, err := config.Load(config.Options{BaseURL: *baseURL, KeyFile: *keyFile})
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		return 1
	}
	client := opencode.New(cfg.APIKey)
	client.BaseURL = cfg.BaseURL

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	usage, err := fetchWithRetry(ctx, client, 3)
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage request failed:", err)
		return 1
	}

	now := time.Now().UTC()
	lengths := metrics.DefaultLengths()
	// Record the sample so one-shot mode also refines the learned window
	// lengths. If a daemon is running it owns the same files; the store's
	// append is line-atomic, and a lost window-length update is harmless.
	if store, err := history.Open(cfg.StateDir); err == nil {
		_ = store.Record(now, usage)
		lengths = store.Lengths()
	}
	snap := metrics.Compute(now, usage, lengths)

	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(snap); err != nil {
			fmt.Fprintln(os.Stderr, "encode:", err)
			return 1
		}
		return 0
	}
	printText(os.Stdout, snap)
	return 0
}

func runDoctor(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	baseURL := fs.String("base-url", "", "API base URL")
	keyFile := fs.String("api-key-file", "", "file containing the API key")
	_ = fs.Parse(args)

	cfg, err := config.Load(config.Options{BaseURL: *baseURL, KeyFile: *keyFile})
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration error:", err)
		return 1
	}
	fmt.Printf("API key source: %s\n", cfg.APIKeySource)
	fmt.Printf("API base URL:   %s\n", cfg.BaseURL)
	fmt.Printf("State dir:      %s\n", cfg.StateDir)

	client := opencode.New(cfg.APIKey)
	client.BaseURL = cfg.BaseURL
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	usage, err := client.FetchUsage(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "usage request failed:", err)
		return 1
	}
	for _, k := range metrics.Order {
		w := metrics.WindowFor(k, usage)
		fmt.Printf("%-8s %3.0f%% used, resets %s\n", k.Title(), w.Percent, w.ResetsAt.Format(time.RFC3339))
	}
	return 0
}

func fetchWithRetry(ctx context.Context, c *opencode.Client, attempts int) (opencode.Usage, error) {
	var lastErr error
	for i := range attempts {
		u, err := c.FetchUsage(ctx)
		if err == nil {
			return u, nil
		}
		lastErr = err
		if opencode.IsKind(err, opencode.KindAuth) {
			return u, err
		}
		select {
		case <-ctx.Done():
			return u, ctx.Err()
		case <-time.After(time.Duration(1<<uint(i)) * 500 * time.Millisecond):
		}
	}
	return opencode.Usage{}, lastErr
}

func printText(w io.Writer, s metrics.Snapshot) {
	writef(w, "OpenCode Go usage — fetched %s\n", s.FetchedAt.Local().Format(time.RFC3339))
	if s.Stale && s.Error != "" {
		writef(w, "  (stale: %s)\n", s.Error)
	}
	for _, k := range s.Order {
		m, ok := s.Windows[string(k)]
		if !ok {
			continue
		}
		reset := metrics.FormatDuration(time.Until(m.ResetsAt))
		writef(w, "  %-8s %6s left (%s used)  resets in %s",
			m.Title, metrics.FormatPercent(m.AvailablePercent), metrics.FormatPercent(m.UsedPercent), reset)
		if m.PaceRatio != nil {
			writef(w, "  pace %.2fx", *m.PaceRatio)
		}
		if m.ExhaustsAt != nil && m.ExhaustsAt.Before(m.ResetsAt) {
			writef(w, "  empty in %s", metrics.FormatDuration(time.Until(*m.ExhaustsAt)))
		}
		writef(w, "\n")
	}
}

// writef writes to w and deliberately ignores the error: for CLI output a
// failure to write is not actionable.
func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func newLogger(level string) *slog.Logger {
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn", "warning":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lv}))
}
