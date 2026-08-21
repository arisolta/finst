package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

const (
	DefaultTimeout = 15 * time.Second
	SECUserAgent   = "finst-cli developer@example.com"
	WebUserAgent   = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

// Client wraps an http.Client with cookie support and retry helpers.
type Client struct {
	HTTPClient *http.Client
	Jar        *cookiejar.Jar
}

// NewClient creates a new HTTP client configured with cookie jar and standard timeouts.
func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		HTTPClient: &http.Client{
			Jar:     jar,
			Timeout: DefaultTimeout,
		},
		Jar: jar,
	}
}

// RequestOptions configures headers and params for an HTTP request.
type RequestOptions struct {
	Headers map[string]string
	Timeout time.Duration
	Retries int
}

// Get performs an HTTP GET request with retries and custom headers.
func (c *Client) Get(ctx context.Context, url string, opts *RequestOptions) ([]byte, error) {
	headers := make(map[string]string)
	retries := 2
	timeout := DefaultTimeout

	if opts != nil {
		for k, v := range opts.Headers {
			headers[k] = v
		}
		if opts.Retries > 0 {
			retries = opts.Retries
		}
		if opts.Timeout > 0 {
			timeout = opts.Timeout
		}
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*300) * time.Millisecond)
		}

		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()

		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return body, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
			continue
		}

		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil, fmt.Errorf("request failed after %d retries: %w", retries, lastErr)
}
