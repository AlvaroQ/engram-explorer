// Package daemon provides an HTTP client for the engram daemon.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrorCode classifies daemon call failures.
type ErrorCode string

const (
	// CodeTimeout indicates the request timed out.
	CodeTimeout ErrorCode = "TIMEOUT"
	// CodeNetwork indicates a network-level error.
	CodeNetwork ErrorCode = "NETWORK"
	// CodeHTTP indicates the daemon returned a non-2xx status.
	CodeHTTP ErrorCode = "HTTP"
	// CodeParse indicates the response body could not be decoded as JSON.
	CodeParse ErrorCode = "PARSE"
)

// DaemonError holds structured error info from a failed daemon request.
type DaemonError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Status  *int      `json:"status,omitempty"`
}

// Result is a discriminated union: either ok with raw JSON data, or an error.
type Result struct {
	OK    bool
	Data  json.RawMessage // non-nil when OK=true
	Error *DaemonError    // non-nil when OK=false
}

// Client is an HTTP client for the engram daemon.
type Client struct {
	baseURL   string
	timeout   time.Duration
	httpClient *http.Client
}

// New creates a Client for the given daemon base URL and timeout.
func New(baseURL string, timeoutMs int) *Client {
	base := strings.TrimRight(baseURL, "/")
	timeout := time.Duration(timeoutMs) * time.Millisecond
	return &Client{
		baseURL: base,
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// FetchJSON calls path on the daemon and returns the raw JSON body.
// On success, Result.OK=true and Result.Data holds the response body bytes.
// On failure, Result.OK=false and Result.Error is populated.
func (c *Client) FetchJSON(path string) Result {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	url := c.baseURL + path

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{
			OK:    false,
			Error: &DaemonError{Code: CodeNetwork, Message: fmt.Sprintf("build request: %v", err)},
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		code := CodeNetwork
		msg := err.Error()
		if ctx.Err() != nil {
			code = CodeTimeout
			msg = "request timed out"
		}
		return Result{OK: false, Error: &DaemonError{Code: code, Message: msg}}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		status := resp.StatusCode
		return Result{
			OK:    false,
			Error: &DaemonError{Code: CodeHTTP, Message: fmt.Sprintf("HTTP %d", resp.StatusCode), Status: &status},
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{OK: false, Error: &DaemonError{Code: CodeParse, Message: fmt.Sprintf("read body: %v", err)}}
	}
	// Validate that it's JSON.
	if !json.Valid(body) {
		return Result{OK: false, Error: &DaemonError{Code: CodeParse, Message: "response is not valid JSON"}}
	}
	return Result{OK: true, Data: body}
}
