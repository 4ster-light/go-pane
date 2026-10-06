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

type fakeProvider struct {
	snap    metrics.Snapshot
	history []history.Sample
}

func (f fakeProvider) Snapshot() metrics.Snapshot         { return f.snap }
func (f fakeProvider) History(limit int) []history.Sample { return f.history }

func sampleSnapshot() metrics.Snapshot {
	return metrics.Snapshot{
		Order:    metrics.Order,
		Headline: metrics.Monthly,
		Windows: map[string]metrics.WindowMetric{
			"monthly": {Kind: metrics.Monthly, Title: "Monthly", UsedPercent: 57, AvailablePercent: 43},
		},
	}
}

func get(t *testing.T, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestHandlerServesUsage(t *testing.T) {
	srv := httptest.NewServer(Handler(fakeProvider{snap: sampleSnapshot()}))
	defer srv.Close()

	resp := get(t, srv.URL+"/v1/usage")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("CORS header = %q, want *", got)
	}

	var snap metrics.Snapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Windows["monthly"].UsedPercent != 57 {
		t.Errorf("monthly used = %v, want 57", snap.Windows["monthly"].UsedPercent)
	}
}

func TestHandlerHealthz(t *testing.T) {
	srv := httptest.NewServer(Handler(fakeProvider{}))
	defer srv.Close()

	if resp := get(t, srv.URL+"/healthz"); resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHandlerRejectsNonLoopback(t *testing.T) {
	h := Handler(fakeProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/usage", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

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
