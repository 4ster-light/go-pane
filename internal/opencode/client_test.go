package opencode

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestFetchUsage(t *testing.T) {
	body, err := os.ReadFile("../../testdata/usage.sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/usage" {
			t.Errorf("path = %q, want /usage", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New("oc_sk_test")
	c.BaseURL = srv.URL
	u, err := c.FetchUsage(context.Background())
	if err != nil {
		t.Fatalf("FetchUsage: %v", err)
	}
	if gotAuth != "Bearer oc_sk_test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if u.Monthly.Percent != 57 {
		t.Errorf("monthly percent = %v, want 57", u.Monthly.Percent)
	}
	want, _ := time.Parse(time.RFC3339, "2026-10-25T14:02:02Z")
	if !u.Monthly.ResetsAt.Equal(want) {
		t.Errorf("monthly resetsAt = %v, want %v", u.Monthly.ResetsAt, want)
	}
}

func TestFetchUsageErrors(t *testing.T) {
	cases := []struct {
		status int
		kind   ErrorKind
	}{
		{http.StatusUnauthorized, KindAuth},
		{http.StatusForbidden, KindAuth},
		{http.StatusTooManyRequests, KindRateLimit},
		{http.StatusBadGateway, KindServer},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(`{"error":{"message":"nope"}}`))
		}))
		client := New("k")
		client.BaseURL = srv.URL
		_, err := client.FetchUsage(context.Background())
		srv.Close()
		if !IsKind(err, c.kind) {
			t.Errorf("status %d: kind = %v, want %v (err=%v)", c.status, err, c.kind, err)
		}
	}
}

func TestFetchUsageDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	c := New("k")
	c.BaseURL = srv.URL
	_, err := c.FetchUsage(context.Background())
	if !IsKind(err, KindDecode) {
		t.Errorf("kind = %v, want decode", err)
	}
}

func TestFetchUsageNoKey(t *testing.T) {
	c := New("")
	_, err := c.FetchUsage(context.Background())
	if !IsKind(err, KindAuth) {
		t.Errorf("kind = %v, want auth", err)
	}
}
