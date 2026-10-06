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
