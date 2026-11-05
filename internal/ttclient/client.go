// Package ttclient is the HTTP client tt-cli uses to talk to a tt server's
// /api/v1. Every failure comes back as an *APIError carrying the server's
// {code, message, details} envelope, or a client-side code for transport
// and auth failures.
package ttclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// Error codes the server sends in its envelope (see httpapi/errors.go).
const (
	CodeValidation    = "validation_failed"
	CodeNotFound      = "not_found"
	CodeConflict      = "conflict"
	CodeSchedulerBusy = "scheduler_busy"
	CodeInternal      = "internal"
)

// Client-side error codes, used where the server never produced an envelope.
const (
	CodeAuth    = "auth_failed"
	CodeNetwork = "network_error"
)

// APIError is the {code, message, details} envelope, whether the server
// sent it or the client synthesised it.
type APIError struct {
	Status  int            `json:"-"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// Client calls one tt instance. The Cloudflare Access headers are sent only
// when both ClientID and ClientSecret are set.
type Client struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	HTTP         *http.Client
}

// New returns a Client that never follows redirects: behind Cloudflare
// Access an unauthenticated request is redirected to a login page, and that
// must surface as an auth error rather than a parsed HTML body.
func New(baseURL, clientID, clientSecret string) *Client {
	return &Client{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTP: &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Do sends body (JSON-encoded when non-nil) to /api/v1+path and decodes a
// JSON response into out (skipped when out is nil or the response is 204).
// The response headers are returned on success.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) (http.Header, error) {
	resp, raw, err := c.send(ctx, method, path, query, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNoContent {
		return resp.Header, nil
	}
	if !isJSON(resp.Header.Get("Content-Type")) {
		return nil, authError(resp, "server returned a non-JSON response (likely an auth or proxy page); check url and Cloudflare Access credentials")
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return nil, &APIError{Status: resp.StatusCode, Code: CodeNetwork, Message: "decode response: " + err.Error()}
		}
	}
	return resp.Header, nil
}

// Fetch GETs /api/v1+path and returns the raw body of a successful response
// whose Content-Type is contentType. Errors decode as in Do.
func (c *Client) Fetch(ctx context.Context, path, contentType string) ([]byte, http.Header, error) {
	resp, raw, err := c.send(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, nil, err
	}
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt != contentType {
		return nil, nil, authError(resp, "server returned an unexpected "+resp.Header.Get("Content-Type")+" response (likely an auth or proxy page); check url and Cloudflare Access credentials")
	}
	return raw, resp.Header, nil
}

// send performs one request and returns the response with its body read.
// Redirects and error statuses come back as an *APIError.
func (c *Client) send(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, []byte, error) {
	u := c.BaseURL + "/api/v1" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("ttclient: encode body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, nil, &APIError{Code: CodeNetwork, Message: err.Error(), Details: map[string]any{"url": c.BaseURL}}
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.ClientID != "" && c.ClientSecret != "" {
		req.Header.Set("CF-Access-Client-Id", c.ClientID)
		req.Header.Set("CF-Access-Client-Secret", c.ClientSecret)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, &APIError{Code: CodeNetwork, Message: err.Error(), Details: map[string]any{"url": c.BaseURL}}
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, &APIError{Code: CodeNetwork, Message: "read response: " + err.Error()}
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, nil, authError(resp, "request was redirected (likely a Cloudflare Access login); check cf_client_id / cf_client_secret")
	}
	if resp.StatusCode >= 400 {
		if !isJSON(resp.Header.Get("Content-Type")) {
			return nil, nil, authError(resp, "server returned a non-JSON response (likely an auth or proxy page); check url and Cloudflare Access credentials")
		}
		var env struct {
			Error *APIError `json:"error"`
		}
		if err := json.Unmarshal(raw, &env); err != nil || env.Error == nil || env.Error.Code == "" {
			return nil, nil, &APIError{Status: resp.StatusCode, Code: CodeNetwork, Message: fmt.Sprintf("unexpected %d response", resp.StatusCode)}
		}
		env.Error.Status = resp.StatusCode
		return nil, nil, env.Error
	}
	return resp, raw, nil
}

func authError(resp *http.Response, msg string) *APIError {
	details := map[string]any{"status": resp.StatusCode}
	if loc := resp.Header.Get("Location"); loc != "" {
		details["location"] = loc
	}
	return &APIError{Status: resp.StatusCode, Code: CodeAuth, Message: msg, Details: details}
}

func isJSON(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	return err == nil && mt == "application/json"
}
