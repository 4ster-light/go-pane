// Package server exposes the daemon state over loopback HTTP.
package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/4ster-light/go-pane/internal/history"
	"github.com/4ster-light/go-pane/internal/metrics"
)

const (
	defaultHistoryLimit = 288
	maxHistoryLimit     = 5000
)

// Provider is the data source the server renders.
type Provider interface {
	Snapshot() metrics.Snapshot
	History(limit int) []history.Sample
}

// New builds the HTTP server.
func New(addr string, p Provider) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           Handler(p),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// Handler returns the loopback-only, CORS-enabled handler. It is exported so
// it can be exercised in tests without binding a socket.
func Handler(p Provider) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /v1/usage", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, p.Snapshot(), true)
	})
	mux.HandleFunc("GET /v1/history", func(w http.ResponseWriter, r *http.Request) {
		limit := defaultHistoryLimit
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= maxHistoryLimit {
				limit = n
			}
		}
		writeJSON(w, map[string]any{"samples": p.History(limit)}, false)
	})
	return cors(loopbackOnly(mux))
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// writeJSON encodes v as JSON. The latest snapshot is indented for curl-friendliness;
// bulkier payloads (history) are written compactly.
func writeJSON(w http.ResponseWriter, v any, indent bool) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	if indent {
		enc.SetIndent("", "  ")
	}
	_ = enc.Encode(v)
}
