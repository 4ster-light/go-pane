package opencode

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestErrorKindString(t *testing.T) {
	cases := map[ErrorKind]string{
		KindUnknown:   "unknown",
		KindNetwork:   "network",
		KindAuth:      "auth",
		KindRateLimit: "rate-limit",
		KindServer:    "server",
		KindDecode:    "decode",
	}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Errorf("ErrorKind(%d).String() = %q, want %q", kind, got, want)
		}
	}
}

func TestErrorFormatting(t *testing.T) {
	cases := []struct {
		name string
		err  *Error
		want string
	}{
		{"bare", &Error{Kind: KindAuth}, "opencode auth error"},
		{"status", &Error{Kind: KindAuth, StatusCode: 401}, "opencode auth error (HTTP 401)"},
		{"status+message", &Error{Kind: KindAuth, StatusCode: 401, Message: "nope"}, "opencode auth error (HTTP 401): nope"},
		{"wrapped", &Error{Kind: KindNetwork, Err: errors.New("boom")}, "opencode network error: boom"},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("%s: Error() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestErrorUnwrap(t *testing.T) {
	base := errors.New("root cause")
	err := &Error{Kind: KindNetwork, Err: base}
	if !errors.Is(err, base) {
		t.Error("errors.Is should reach the wrapped error")
	}
}

func TestServerMessage(t *testing.T) {
	if got := serverMessage([]byte(`{"error":{"message":"nope"}}`)); got != "nope" {
		t.Errorf("structured message = %q, want nope", got)
	}
	if got := serverMessage([]byte("  plain body  \n")); got != "plain body" {
		t.Errorf("plain body = %q, want %q", got, "plain body")
	}
	long := strings.Repeat("é", 300)
	got := serverMessage([]byte(long))
	if n := utf8.RuneCountInString(got); n != 200 {
		t.Errorf("truncated runes = %d, want 200", n)
	}
	if !utf8.ValidString(got) {
		t.Error("truncated message is not valid UTF-8")
	}
}

func TestNewDefaults(t *testing.T) {
	c := New("oc_sk_k")
	if c.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, DefaultBaseURL)
	}
	if c.HTTP == nil || c.HTTP.Timeout <= 0 {
		t.Error("HTTP client should have a positive timeout")
	}
	if c.UserAgent == "" {
		t.Error("UserAgent should be set")
	}
	if c.APIKey != "oc_sk_k" {
		t.Errorf("APIKey = %q", c.APIKey)
	}
}

func TestFetchUsageNetworkError(t *testing.T) {
	c := New("k")
	c.BaseURL = "http://127.0.0.1:1" // nothing listens here
	if _, err := c.FetchUsage(context.Background()); !IsKind(err, KindNetwork) {
		t.Errorf("kind = %v, want network (err=%v)", err, err)
	}
}

func TestFetchUsageContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := New("k")
	c.BaseURL = srv.URL
	if _, err := c.FetchUsage(ctx); !IsKind(err, KindNetwork) {
		t.Errorf("kind = %v, want network (err=%v)", err, err)
	}
}

func TestFetchUsageUnknownStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	c := New("k")
	c.BaseURL = srv.URL
	if _, err := c.FetchUsage(context.Background()); !IsKind(err, KindUnknown) {
		t.Errorf("kind = %v, want unknown (err=%v)", err, err)
	}
}

func TestFetchUsageEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New("k")
	c.BaseURL = srv.URL
	if _, err := c.FetchUsage(context.Background()); !IsKind(err, KindDecode) {
		t.Errorf("kind = %v, want decode (err=%v)", err, err)
	}
}

func TestErrorStatusCodeInMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	c := New("k")
	c.BaseURL = srv.URL
	_, err := c.FetchUsage(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "bad key") || !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want message and status", err)
	}
	var e *Error
	if !errors.As(err, &e) || e.StatusCode != http.StatusUnauthorized {
		t.Errorf("typed error = %+v, want status 401", e)
	}
	if e.Unwrap() != nil {
		t.Errorf("Unwrap() = %v, want nil", e.Unwrap())
	}
}
