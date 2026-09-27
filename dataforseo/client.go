// Package dataforseo is a small client for the DataForSEO v3 API. It handles
// authentication, the response envelope, retries of server errors, and the
// task status rules. Endpoint-specific request and result types belong to the
// callers.
package dataforseo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// DefaultBaseURL is the production DataForSEO API.
const DefaultBaseURL = "https://api.dataforseo.com"

const (
	defaultTimeout   = 60 * time.Second
	maxServerRetries = 2
	retryBackoff     = 250 * time.Millisecond
	maxErrorBody     = 1600
	maxResponseBytes = 64 << 20
)

// Status codes DataForSEO uses in the response and task envelopes.
const (
	StatusOK       = 20000
	StatusTaskSent = 20100
)

// upstreamStatusCodes are task failures on DataForSEO's side, not caused by the
// request. A caller may retry them later.
var upstreamStatusCodes = map[int]bool{
	40101: true, // Internal SE Server Error.
	40103: true, // Task execution failed, please try to resubmit.
	50000: true, // Internal Error.
	50301: true, // 3rd Party API Service Unavailable.
	50302: true, // Internal 3rd Party API Service Unavailable.
	50303: true, // Update in progress. Please try after a few minutes.
	50304: true, // This function temporarily unavailable.
	50401: true, // Internal Error - Timeout.
	50402: true, // Target page took too long to respond.
}

// inProgressStatusCodes mean a posted task has no result yet.
var inProgressStatusCodes = map[int]bool{20100: true, 40601: true, 40602: true}

var invalidFieldRe = regexp.MustCompile(`(?i)Invalid Field:\s*'([^']+)'`)

// Client calls the DataForSEO API. The zero value is not usable; set APIKey or
// use New or NewWithLogin.
type Client struct {
	// APIKey is base64("login:password"), the credential DataForSEO's
	// dashboard shows for Basic authentication.
	APIKey string
	// BaseURL overrides DefaultBaseURL, for tests or the sandbox API.
	BaseURL string
	// HTTPClient overrides the default client, which has a 60 s timeout.
	HTTPClient *http.Client
}

// New returns a client for a base64 "login:password" API key.
func New(apiKey string) *Client {
	return &Client{APIKey: strings.TrimSpace(apiKey)}
}

// NewWithLogin returns a client for a DataForSEO login and API password.
func NewWithLogin(login, password string) *Client {
	return New(base64.StdEncoding.EncodeToString([]byte(login + ":" + password)))
}

// Task is one task of a DataForSEO response. Result keeps the raw result
// array; decode it with FirstResult or json.Unmarshal.
type Task struct {
	ID            string          `json:"id"`
	StatusCode    int             `json:"status_code"`
	StatusMessage string          `json:"status_message"`
	Cost          float64         `json:"cost"`
	ResultCount   int             `json:"result_count"`
	Path          []string        `json:"path"`
	Data          json.RawMessage `json:"data"`
	Result        json.RawMessage `json:"result"`
}

// FirstResult decodes the first element of the task's result array into v. It
// returns false, with v unchanged, when the result is null or empty.
func (t *Task) FirstResult(v any) (bool, error) {
	if len(t.Result) == 0 || string(t.Result) == "null" {
		return false, nil
	}
	var results []json.RawMessage
	if err := json.Unmarshal(t.Result, &results); err != nil {
		return false, fmt.Errorf("dataforseo: decode result: %w", err)
	}
	if len(results) == 0 || string(results[0]) == "null" {
		return false, nil
	}
	if err := json.Unmarshal(results[0], v); err != nil {
		return false, fmt.Errorf("dataforseo: decode result: %w", err)
	}
	return true, nil
}

type envelope struct {
	StatusCode    int     `json:"status_code"`
	StatusMessage string  `json:"status_message"`
	Cost          float64 `json:"cost"`
	Tasks         []Task  `json:"tasks"`
}

// Error is a failed DataForSEO call: an HTTP error, a failed response
// envelope, or a failed task.
type Error struct {
	// HTTPStatus is set for a non-2xx HTTP response.
	HTTPStatus int
	// StatusCode is the DataForSEO status code of the response or the task.
	StatusCode int
	Message    string
	// Path is the API path, for example /v3/dataforseo_labs/google/ranked_keywords/live.
	Path string
	// Cost is the USD amount DataForSEO charged for the failed task, if any.
	Cost float64
}

func (e *Error) Error() string {
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("dataforseo: HTTP %d on %s: %s", e.HTTPStatus, e.Path, e.Message)
	}
	return fmt.Sprintf("dataforseo: %s (status %d on %s)", e.Message, e.StatusCode, e.Path)
}

// Upstream reports a failure on DataForSEO's side (HTTP 5xx or a provider
// task error) that the caller did not cause.
func (e *Error) Upstream() bool {
	return e.HTTPStatus >= 500 || upstreamStatusCodes[e.StatusCode]
}

// RateLimited reports an HTTP 429 from DataForSEO.
func (e *Error) RateLimited() bool { return e.HTTPStatus == http.StatusTooManyRequests }

// AuthFailed reports rejected credentials.
func (e *Error) AuthFailed() bool {
	return e.HTTPStatus == http.StatusUnauthorized || e.StatusCode == 40100
}

// InvalidField reports a task rejected because of one request field.
func (e *Error) InvalidField() bool { return invalidFieldRe.MatchString(e.Message) }

// NoResults reports a task that ran but found no search results. Most callers
// treat it as an empty answer.
func (e *Error) NoResults() bool {
	return strings.Contains(strings.ToLower(e.Message), "no search results")
}

// InProgress reports a posted task whose result is not ready yet.
func (e *Error) InProgress() bool { return inProgressStatusCodes[e.StatusCode] }

// IsNoResults reports whether err is a DataForSEO "no search results" task.
func IsNoResults(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.NoResults()
}

// Post sends one task to a POST endpoint and returns that task after the
// envelope checks. A task that failed returns an *Error.
func (c *Client) Post(ctx context.Context, path string, task any) (*Task, error) {
	body, err := json.Marshal([]any{task})
	if err != nil {
		return nil, fmt.Errorf("dataforseo: encode task: %w", err)
	}
	return c.do(ctx, http.MethodPost, path, body, StatusOK)
}

// PostTask sends one task to an asynchronous task_post endpoint. The returned
// task's ID is what the matching task_get endpoint takes.
func (c *Client) PostTask(ctx context.Context, path string, task any) (*Task, error) {
	body, err := json.Marshal([]any{task})
	if err != nil {
		return nil, fmt.Errorf("dataforseo: encode task: %w", err)
	}
	return c.do(ctx, http.MethodPost, path, body, StatusTaskSent)
}

// Get calls a GET endpoint, such as a task_get endpoint, and returns its first
// task after the envelope checks.
func (c *Client) Get(ctx context.Context, path string) (*Task, error) {
	return c.do(ctx, http.MethodGet, path, nil, StatusOK)
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, okTaskStatus int) (*Task, error) {
	raw, err := c.send(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("dataforseo: decode response from %s: %w", path, err)
	}
	if env.StatusCode != StatusOK {
		msg := env.StatusMessage
		if msg == "" {
			msg = "request failed"
		}
		return nil, &Error{StatusCode: env.StatusCode, Message: msg, Path: path}
	}
	if len(env.Tasks) == 0 {
		return nil, &Error{StatusCode: env.StatusCode, Message: "response has no task", Path: path}
	}
	task := &env.Tasks[0]
	if task.StatusCode != okTaskStatus {
		msg := task.StatusMessage
		if msg == "" {
			msg = "task failed"
		}
		return nil, &Error{
			StatusCode: task.StatusCode,
			Message:    describeInvalidField(msg, task.Data),
			Path:       path,
			Cost:       task.Cost,
		}
	}
	return task, nil
}

// send performs the HTTP exchange, retrying 5xx responses twice.
func (c *Client) send(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if c.APIKey == "" {
		return nil, errors.New("dataforseo: API key is empty")
	}
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: defaultTimeout}
	}
	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, base+path, reader)
		if err != nil {
			return nil, fmt.Errorf("dataforseo: build request: %w", err)
		}
		req.Header.Set("Authorization", "Basic "+c.APIKey)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := hc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("dataforseo: %s: %w", path, err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("dataforseo: read %s: %w", path, readErr)
		}
		if resp.StatusCode/100 == 2 {
			return raw, nil
		}
		if resp.StatusCode >= 500 && attempt < maxServerRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryBackoff * time.Duration(attempt+1)):
			}
			continue
		}
		msg := strings.TrimSpace(string(raw))
		if len(msg) > maxErrorBody {
			msg = msg[:maxErrorBody] + "... [truncated]"
		}
		return nil, &Error{HTTPStatus: resp.StatusCode, Message: msg, Path: path}
	}
}

// describeInvalidField appends the value that was sent for the rejected field,
// so a caller can see which input DataForSEO refused.
func describeInvalidField(msg string, data json.RawMessage) string {
	m := invalidFieldRe.FindStringSubmatch(msg)
	if m == nil || len(data) == 0 {
		return msg
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return msg
	}
	value, ok := fields[m[1]]
	if !ok {
		return msg
	}
	return fmt.Sprintf("%s (sent %s=%s)", msg, m[1], value)
}
