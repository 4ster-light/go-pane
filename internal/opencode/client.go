// Package opencode is a minimal client for the OpenCode Go usage API.
package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the verified usage API base.
// The endpoint is GET {BaseURL}/usage.
const DefaultBaseURL = "https://opencode.ai/zen/go/v1"

// Window is one usage window returned by the API.
type Window struct {
	Status   string    `json:"status"`
	Percent  float64   `json:"percent"`
	ResetsAt time.Time `json:"resetsAt"`
}

// Usage is the full usage payload.
type Usage struct {
	Rolling Window `json:"rolling"`
	Weekly  Window `json:"weekly"`
	Monthly Window `json:"monthly"`
}

type usageEnvelope struct {
	Usage Usage `json:"usage"`
}

// ErrorKind classifies API failures so callers can react appropriately.
type ErrorKind int

const (
	KindUnknown ErrorKind = iota
	KindNetwork
	KindAuth
	KindRateLimit
	KindServer
	KindDecode
)

func (k ErrorKind) String() string {
	switch k {
	case KindNetwork:
		return "network"
	case KindAuth:
		return "auth"
	case KindRateLimit:
		return "rate-limit"
	case KindServer:
		return "server"
	case KindDecode:
		return "decode"
	default:
		return "unknown"
	}
}

// Error is a typed API error.
type Error struct {
	Kind       ErrorKind
	StatusCode int
	Message    string
	Err        error
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" && e.Err != nil {
		msg = e.Err.Error()
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("opencode %s error (HTTP %d): %s", e.Kind, e.StatusCode, msg)
	}
	return fmt.Sprintf("opencode %s error: %s", e.Kind, msg)
}

func (e *Error) Unwrap() error { return e.Err }

// IsKind reports whether err is an *Error of the given kind.
func IsKind(err error, kind ErrorKind) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind == kind
	}
	return false
}

// Client talks to the OpenCode Go API.
type Client struct {
	HTTP      *http.Client
	BaseURL   string
	APIKey    string
	UserAgent string
}

// New returns a client with sane defaults.
func New(apiKey string) *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		BaseURL:   DefaultBaseURL,
		APIKey:    apiKey,
		UserAgent: "go-pane/0.1 (+https://github.com/4ster-light/go-pane)",
	}
}

// FetchUsage performs a single usage request.
func (c *Client) FetchUsage(ctx context.Context) (Usage, error) {
	var out Usage
	if strings.TrimSpace(c.APIKey) == "" {
		return out, &Error{Kind: KindAuth, Message: "no API key configured"}
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/usage"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return out, &Error{Kind: KindUnknown, Message: "build request", Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return out, &Error{Kind: KindNetwork, Message: "request failed", Err: err}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, &Error{Kind: KindNetwork, Message: "read body", Err: err}
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return out, &Error{Kind: KindAuth, StatusCode: resp.StatusCode, Message: serverMessage(body)}
	case resp.StatusCode == http.StatusTooManyRequests:
		return out, &Error{Kind: KindRateLimit, StatusCode: resp.StatusCode, Message: serverMessage(body)}
	case resp.StatusCode >= 500:
		return out, &Error{Kind: KindServer, StatusCode: resp.StatusCode, Message: serverMessage(body)}
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return out, &Error{Kind: KindUnknown, StatusCode: resp.StatusCode, Message: serverMessage(body)}
	}

	var env usageEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return out, &Error{Kind: KindDecode, StatusCode: resp.StatusCode, Message: "invalid JSON", Err: err}
	}
	return env.Usage, nil
}

func serverMessage(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
