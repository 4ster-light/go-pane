package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/4ster-light/go-pane/internal/history"
	"github.com/4ster-light/go-pane/internal/metrics"
)

// recordingProvider captures the limit the handler forwards to History.
type recordingProvider struct {
	samples []history.Sample
	limit   int
}

func (p *recordingProvider) Snapshot() metrics.Snapshot { return sampleSnapshot() }
func (p *recordingProvider) History(limit int) []history.Sample {
	p.limit = limit
	return p.samples
}

func TestHandlerHistoryDefaultLimit(t *testing.T) {
	p := &recordingProvider{samples: []history.Sample{{Percents: map[string]float64{"rolling": 3}}}}
	srv := httptest.NewServer(Handler(p))
	defer srv.Close()

	resp := get(t, srv.URL+"/v1/history")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		Samples []history.Sample `json:"samples"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Samples) != 1 {
		t.Errorf("samples = %d, want 1", len(body.Samples))
	}
	if p.limit != defaultHistoryLimit {
		t.Errorf("limit = %d, want %d", p.limit, defaultHistoryLimit)
	}
}

func TestHandlerHistoryLimit(t *testing.T) {
	p := &recordingProvider{}
	srv := httptest.NewServer(Handler(p))
	defer srv.Close()

	if resp := get(t, srv.URL+"/v1/history?limit=2"); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if p.limit != 2 {
		t.Errorf("limit = %d, want 2", p.limit)
	}
}

func TestHandlerHistoryLimitInvalidFallsBack(t *testing.T) {
	for _, q := range []string{"0", "-1", "abc", "99999"} {
		p := &recordingProvider{}
		srv := httptest.NewServer(Handler(p))
		resp := get(t, srv.URL+"/v1/history?limit="+q)
		_ = resp.Body.Close()
		srv.Close()
		if p.limit != defaultHistoryLimit {
			t.Errorf("limit=%q -> %d, want default %d", q, p.limit, defaultHistoryLimit)
		}
	}
}

func TestHandlerOptionsPreflight(t *testing.T) {
	h := Handler(&recordingProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/v1/usage", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("CORS header = %q, want *", got)
	}
}

func TestHandlerRejectsNonGet(t *testing.T) {
	h := Handler(&recordingProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/usage", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandlerRejectsBadRemoteAddr(t *testing.T) {
	h := Handler(&recordingProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/usage", nil)
	req.RemoteAddr = "not-a-host-port"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestNewServerTimeouts(t *testing.T) {
	srv := New("127.0.0.1:0", &recordingProvider{})
	if srv.ReadHeaderTimeout <= 0 || srv.ReadTimeout <= 0 || srv.WriteTimeout <= 0 || srv.IdleTimeout <= 0 {
		t.Errorf("server timeouts must all be positive: %+v", srv)
	}
}
