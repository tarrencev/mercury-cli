package mercuryhttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type ClientOptions struct {
	Timeout            time.Duration
	Debug              bool
	Trace              bool
	RetryNonIdempotent bool
	UserAgent          string
	Out                io.Writer
}

type Client struct {
	http *http.Client
	opts ClientOptions
}

type Result struct {
	Status  int
	Headers http.Header
	Body    []byte
}

func NewClient(opts ClientOptions) (*Client, error) {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 30 * time.Second
	}
	return &Client{
		http: &http.Client{
			Timeout: opts.Timeout,
		},
		opts: opts,
	}, nil
}

func (c *Client) Do(req *http.Request, reqBody []byte) (*Result, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	ctx := req.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Apply headers.
	if c.opts.UserAgent != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/json")
	}

	if c.opts.Debug || c.opts.Trace {
		c.logRequest(req, reqBody)
	}

	const maxAttempts = 5
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			if req.GetBody != nil {
				rc, err := req.GetBody()
				if err == nil {
					req.Body = rc
				}
			} else if len(reqBody) > 0 {
				req.Body = io.NopCloser(bytes.NewReader(reqBody))
			}
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			break
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		if c.opts.Debug || c.opts.Trace {
			c.logResponse(resp, body)
		}

		if shouldRetry(resp.StatusCode, req.Method, c.opts.RetryNonIdempotent) && attempt < maxAttempts {
			sleep := retryBackoff(resp, attempt)
			select {
			case <-time.After(sleep):
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		return &Result{
			Status:  resp.StatusCode,
			Headers: resp.Header.Clone(),
			Body:    body,
		}, nil
	}

	if lastErr == nil {
		lastErr = errors.New("request failed")
	}
	return nil, lastErr
}

func applyAuth(req *http.Request, token string, scheme string) {
	switch scheme {
	case "basic":
		// The Mercury OpenAPI uses HTTP Basic with the API token as username and blank password.
		raw := token + ":"
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(raw)))
	default:
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// ApplyAuth sets the Authorization header on req using the provided scheme.
// Supported schemes:
// - bearer: Authorization: Bearer <token>
// - basic:  Authorization: Basic base64(<token>:)
func ApplyAuth(req *http.Request, token string, scheme string) {
	applyAuth(req, token, scheme)
}

func shouldRetry(status int, method string, retryNonIdempotent bool) bool {
	if status == http.StatusTooManyRequests || status >= 500 {
		switch strings.ToUpper(method) {
		case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
			return true
		default:
			return retryNonIdempotent
		}
	}
	return false
}

func retryBackoff(resp *http.Response, attempt int) time.Duration {
	if resp != nil {
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			// Retry-After can be an integer seconds or a HTTP date.
			if secs, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil && secs >= 0 {
				return time.Duration(secs) * time.Second
			}
			if t, err := http.ParseTime(ra); err == nil {
				d := time.Until(t)
				if d > 0 {
					return d
				}
			}
		}
	}

	// Exponential backoff with jitter: 200ms * 2^(attempt-1), capped at 5s.
	base := 200 * time.Millisecond
	d := base * (1 << (attempt - 1))
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	// +/- 50% jitter
	j := time.Duration(rand.Int63n(int64(d))) - d/2
	return d + j
}

func (c *Client) logRequest(req *http.Request, body []byte) {
	fmt.Fprintf(c.opts.Out, "> %s %s\n", req.Method, req.URL.String())
	for k, vv := range req.Header {
		v := strings.Join(vv, ", ")
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Proxy-Authorization") {
			v = "<redacted>"
		}
		fmt.Fprintf(c.opts.Out, "> %s: %s\n", k, v)
	}
	if c.opts.Trace && len(body) > 0 {
		fmt.Fprintf(c.opts.Out, ">\n")
		_, _ = c.opts.Out.Write(body)
		if body[len(body)-1] != '\n' {
			_, _ = c.opts.Out.Write([]byte("\n"))
		}
	}
}

func (c *Client) logResponse(resp *http.Response, body []byte) {
	if resp == nil {
		return
	}
	fmt.Fprintf(c.opts.Out, "< %s\n", resp.Status)
	if c.opts.Debug {
		ct := resp.Header.Get("Content-Type")
		if ct != "" {
			fmt.Fprintf(c.opts.Out, "< Content-Type: %s\n", ct)
		}
		fmt.Fprintf(c.opts.Out, "< Content-Length: %d\n", len(body))
	}
	if c.opts.Trace && len(body) > 0 {
		fmt.Fprintf(c.opts.Out, "<\n")
		_, _ = c.opts.Out.Write(body)
		if body[len(body)-1] != '\n' {
			_, _ = c.opts.Out.Write([]byte("\n"))
		}
	}
}
