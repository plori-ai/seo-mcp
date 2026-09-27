package dataforseo_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

const okResponse = `{"status_code":20000,"tasks":[{"id":"sample-task","status_code":20000,"cost":0.01,"result_count":1,"result":[{"value":7}]}]}`

func TestClientAuthenticationAndRequest(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("sample-login:sample-password"))
	tests := []struct {
		name string
		new  func() *dataforseo.Client
	}{
		{"API key", func() *dataforseo.Client { return dataforseo.New(key) }},
		{"trim API key", func() *dataforseo.Client { return dataforseo.New(" \n" + key + "\n") }},
		{"login and password", func() *dataforseo.Client { return dataforseo.NewWithLogin("sample-login", "sample-password") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v3/sample/live" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Basic "+key {
					t.Error("incorrect Basic authorization header")
				}
				for _, header := range []string{"Content-Type", "Accept"} {
					if r.Header.Get(header) != "application/json" {
						t.Errorf("%s = %q", header, r.Header.Get(header))
					}
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if string(body) != `[{"keyword":"sample keyword","location_code":2840}]` {
					t.Errorf("request body = %s", body)
				}
				_, _ = io.WriteString(w, okResponse)
			}))
			defer fake.Close()
			client := tt.new()
			client.BaseURL = fake.URL + "/"
			task, err := client.Post(t.Context(), "/v3/sample/live", map[string]any{"keyword": "sample keyword", "location_code": 2840})
			if err != nil {
				t.Fatal(err)
			}
			if task.ID != "sample-task" || task.Cost != 0.01 || task.ResultCount != 1 {
				t.Errorf("task = %+v", task)
			}
		})
	}
}

func TestClientEnvelopeAndTaskFailures(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		statusCode int
		cost       float64
		message    string
		invalid    bool
	}{
		{"envelope", `{"status_code":40100,"status_message":"Authentication failed.","tasks":[]}`, 40100, 0, "Authentication failed.", false},
		{"empty envelope message", `{"status_code":50000,"tasks":[]}`, 50000, 0, "request failed", false},
		{"no task", `{"status_code":20000,"tasks":[]}`, 20000, 0, "response has no task", false},
		{"task with cost", `{"status_code":20000,"tasks":[{"status_code":50000,"status_message":"Internal Error.","cost":0.025}]}`, 50000, 0.025, "Internal Error.", false},
		{"empty task message", `{"status_code":20000,"tasks":[{"status_code":40501,"cost":0.01}]}`, 40501, 0.01, "task failed", false},
		{"invalid field detail", `{"status_code":20000,"tasks":[{"status_code":40501,"status_message":"Invalid Field: 'location_code'.","cost":0.005,"data":{"location_code":99999}}]}`, 40501, 0.005, "Invalid Field: 'location_code'. (sent location_code=99999)", true},
		{"invalid string field", `{"status_code":20000,"tasks":[{"status_code":40501,"status_message":"Invalid Field: 'language_code'.","data":{"language_code":"invalid"}}]}`, 40501, 0, `Invalid Field: 'language_code'. (sent language_code="invalid")`, true},
		{"invalid field missing data", `{"status_code":20000,"tasks":[{"status_code":40501,"status_message":"Invalid Field: 'location_code'.","data":null}]}`, 40501, 0, "Invalid Field: 'location_code'.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, tt.response)
			}))
			defer fake.Close()
			client := dataforseo.New("synthetic-api-key")
			client.BaseURL = fake.URL
			task, err := client.Post(t.Context(), "/v3/sample/live", map[string]any{})
			var provider *dataforseo.Error
			if task != nil || !errors.As(err, &provider) {
				t.Fatalf("got (%+v, %v), want *dataforseo.Error", task, err)
			}
			if provider.StatusCode != tt.statusCode || provider.HTTPStatus != 0 || provider.Cost != tt.cost || provider.Path != "/v3/sample/live" || provider.Message != tt.message || provider.InvalidField() != tt.invalid {
				t.Errorf("error = %+v", provider)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("calls = %d, want 1 (do not retry envelope/task failures)", got)
			}
		})
	}
}

func TestClientRetries5xx(t *testing.T) {
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != `[{"keyword":"sample"}]` {
			t.Errorf("attempt %d body = %q, error = %v", attempt, body, err)
		}
		if attempt < 3 {
			http.Error(w, "temporary upstream error", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, okResponse)
	}))
	defer fake.Close()
	client := dataforseo.New("synthetic-api-key")
	client.BaseURL = fake.URL
	task, err := client.Post(t.Context(), "/v3/sample/live", map[string]string{"keyword": "sample"})
	if err != nil || task == nil || task.ID != "sample-task" {
		t.Fatalf("retry result = (%+v, %v)", task, err)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestClientDoesNotRetry4xx(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				http.Error(w, "request rejected", status)
			}))
			defer fake.Close()
			client := dataforseo.New("synthetic-api-key")
			client.BaseURL = fake.URL
			_, err := client.Post(t.Context(), "/v3/sample/live", map[string]string{})
			var provider *dataforseo.Error
			if !errors.As(err, &provider) || provider.HTTPStatus != status {
				t.Fatalf("error = %v, want HTTP %d", err, status)
			}
			if provider.RateLimited() != (status == 429) || provider.AuthFailed() != (status == 401) || provider.Upstream() {
				t.Errorf("wrong HTTP classification: %+v", provider)
			}
			if got := calls.Load(); got != 1 {
				t.Errorf("calls = %d, want 1", got)
			}
		})
	}
}

func TestClientNoResults(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"status_code":20000,"tasks":[{"status_code":40102,"status_message":"No Search Results.","cost":0.001,"result":null}]}`)
	}))
	defer fake.Close()
	client := dataforseo.New("synthetic-api-key")
	client.BaseURL = fake.URL
	_, err := client.Post(t.Context(), "/v3/sample/live", map[string]string{})
	var provider *dataforseo.Error
	if !errors.As(err, &provider) || !provider.NoResults() || provider.Cost != 0.001 {
		t.Fatalf("error = %v, want charged no-results error", err)
	}
	if !dataforseo.IsNoResults(fmt.Errorf("wrapped: %w", err)) {
		t.Error("IsNoResults did not unwrap provider error")
	}
	for _, other := range []error{nil, errors.New("no search results"), &dataforseo.Error{Message: "task failed"}} {
		if dataforseo.IsNoResults(other) {
			t.Errorf("IsNoResults(%v) = true", other)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type closeSignalBody struct {
	io.ReadCloser
	closed chan struct{}
}

func (b *closeSignalBody) Close() error {
	err := b.ReadCloser.Close()
	close(b.closed)
	return err
}

func TestClientCancelDuringBackoff(t *testing.T) {
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "temporary upstream error", http.StatusServiceUnavailable)
	}))
	defer fake.Close()
	client := dataforseo.New("synthetic-api-key")
	client.BaseURL = fake.URL
	client.HTTPClient = fake.Client()
	transport := client.HTTPClient.Transport
	bodyClosed := make(chan struct{})
	client.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		response, err := transport.RoundTrip(r)
		if err == nil && calls.Load() == 1 {
			response.Body = &closeSignalBody{ReadCloser: response.Body, closed: bodyClosed}
		}
		return response, err
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := client.Post(ctx, "/v3/sample/live", map[string]string{})
		done <- err
	}()
	select {
	case <-bodyClosed:
		// The response is consumed; the client now waits before its next attempt.
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("client did not consume the first 503 response")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("client did not stop its retry backoff")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}

func TestTaskFirstResult(t *testing.T) {
	tests := []struct {
		name    string
		result  string
		found   bool
		want    map[string]int
		wantErr bool
	}{
		{"absent", "", false, map[string]int{"unchanged": 1}, false},
		{"null", "null", false, map[string]int{"unchanged": 1}, false},
		{"empty", "[]", false, map[string]int{"unchanged": 1}, false},
		{"null item", "[null]", false, map[string]int{"unchanged": 1}, false},
		{"one", `[{"value":7}]`, true, map[string]int{"unchanged": 1, "value": 7}, false},
		{"first of two", `[{"value":7},{"value":9}]`, true, map[string]int{"unchanged": 1, "value": 7}, false},
		{"invalid JSON", "[", false, nil, true},
		{"wrong item type", `["invalid"]`, false, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &dataforseo.Task{Result: json.RawMessage(tt.result)}
			value := map[string]int{"unchanged": 1}
			found, err := task.FirstResult(&value)
			if (err != nil) != tt.wantErr || found != tt.found {
				t.Fatalf("FirstResult = (%v, %v), want (%v, error=%v)", found, err, tt.found, tt.wantErr)
			}
			if !tt.wantErr && !reflect.DeepEqual(value, tt.want) {
				t.Errorf("value = %v, want %v", value, tt.want)
			}
		})
	}
}

func TestClientPostTaskAndGet(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/sample/task_post":
			body, err := io.ReadAll(r.Body)
			if r.Method != http.MethodPost || err != nil || string(body) != `[{"keyword":"sample"}]` {
				t.Errorf("task_post: method=%s body=%s error=%v", r.Method, body, err)
			}
			_, _ = io.WriteString(w, `{"status_code":20000,"tasks":[{"id":"sample-task","status_code":20100,"cost":0.01,"result":null}]}`)
		case "/v3/sample/task_get/sample-task":
			body, err := io.ReadAll(r.Body)
			if r.Method != http.MethodGet || err != nil || len(body) != 0 {
				t.Errorf("task_get: method=%s body=%s error=%v", r.Method, body, err)
			}
			_, _ = io.WriteString(w, okResponse)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer fake.Close()
	client := dataforseo.New("synthetic-api-key")
	client.BaseURL = fake.URL
	posted, err := client.PostTask(t.Context(), "/v3/sample/task_post", map[string]string{"keyword": "sample"})
	if err != nil || posted == nil || posted.ID != "sample-task" || posted.StatusCode != dataforseo.StatusTaskSent {
		t.Fatalf("PostTask = (%+v, %v)", posted, err)
	}
	result, err := client.Get(t.Context(), "/v3/sample/task_get/"+posted.ID)
	if err != nil || result == nil || result.StatusCode != dataforseo.StatusOK {
		t.Fatalf("Get = (%+v, %v)", result, err)
	}
	for _, code := range []int{20100, 40601, 40602} {
		if !(&dataforseo.Error{StatusCode: code}).InProgress() {
			t.Errorf("status %d should be in progress", code)
		}
	}
	if (&dataforseo.Error{StatusCode: 50000}).InProgress() {
		t.Error("failed task must not be in progress")
	}
}

func TestClientRejectsMalformedResponse(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "not JSON")
	}))
	defer fake.Close()
	client := dataforseo.New("synthetic-api-key")
	client.BaseURL = fake.URL
	_, err := client.Get(t.Context(), "/v3/sample/task_get/id")
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("error = %v, want response decoding error", err)
	}
}
