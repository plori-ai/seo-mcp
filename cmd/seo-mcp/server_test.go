package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
	"github.com/plori-ai/seo-mcp/toolset"
)

func connectTestClient(t *testing.T, server *mcp.Server) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "dev"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return ctx, cs
}

func TestMCPListAndCall(t *testing.T) {
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v3/dataforseo_labs/google/ranked_keywords/live" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected endpoint", http.StatusBadRequest)
			return
		}
		var tasks []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&tasks); err != nil || len(tasks) != 1 {
			t.Errorf("decode tasks: %v (count %d)", err, len(tasks))
			http.Error(w, "invalid task", http.StatusBadRequest)
			return
		}
		wantTask := map[string]any{
			"target":        "example.com",
			"location_code": float64(2826),
			"language_code": "en",
			"limit":         float64(1),
			"order_by":      []any{"keyword_data.keyword_info.search_volume,desc"},
		}
		if !reflect.DeepEqual(tasks[0], wantTask) {
			t.Errorf("task = %#v, want %#v", tasks[0], wantTask)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status_code":20000,"tasks":[{"status_code":20000,"cost":0.01,"result":[{"total_count":1,"items":[{"keyword_data":{"keyword":"sample keyword","keyword_info":{"search_volume":20}},"ranked_serp_element":{"serp_item":{"rank_group":3,"url":"https://example.com/"}}}]}]}]}`)
	}))
	defer fake.Close()
	api := dataforseo.New("synthetic-api-key")
	api.BaseURL = fake.URL
	client := seo.New(api, seo.WithDefaultMarket(seo.Market{LocationCode: 2826, LanguageCode: "en"}))
	ctx, session := connectTestClient(t, newServer(toolset.New(client)))

	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	definitions := toolset.Tools()
	if len(listed.Tools) != 5 || len(listed.Tools) != len(definitions) {
		t.Fatalf("listed %d tools, want all five", len(listed.Tools))
	}
	byName := make(map[string]*mcp.Tool)
	for _, tool := range listed.Tools {
		byName[tool.Name] = tool
	}
	for _, definition := range definitions {
		tool := byName[definition.Name]
		if tool == nil {
			t.Fatalf("missing tool %s", definition.Name)
		}
		if tool.Title != definition.Title || tool.Description != definition.Description {
			t.Errorf("changed metadata for %s", definition.Name)
		}
		var schema any
		if err := json.Unmarshal(definition.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(tool.InputSchema, schema) {
			t.Errorf("changed schema for %s", definition.Name)
		}
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || !*tool.Annotations.OpenWorldHint {
			t.Errorf("missing read-only/open-world annotations for %s", definition.Name)
		}
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_ranked_keywords",
		Arguments: json.RawMessage(`{"target":"example.com","scope":"subdomains","limit":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want text", result.Content[0])
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(text.Text), &body); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.StructuredContent, body) {
		t.Error("structured content differs from JSON text content")
	}
	keywords, ok := body["keywords"].([]any)
	if !ok || len(keywords) != 1 || body["totalCount"] != float64(1) || body["target"] != "example.com" || body["scope"] != "subdomains" {
		t.Errorf("unexpected result shape: %s", text.Text)
	}
	if _, exists := body["meta"]; exists {
		t.Error("unexpected OpenSEO project metadata")
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("DataForSEO calls = %d, want 1", got)
	}

	invalid, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_ranked_keywords", Arguments: map[string]any{"target": ""}})
	if err != nil || invalid == nil || !invalid.IsError {
		t.Fatalf("invalid input = (%+v, %v), want tool error", invalid, err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("invalid input reached DataForSEO: %d calls", got)
	}
}

func TestMCPProviderError(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprint(w, "private upstream response")
	}))
	defer fake.Close()
	api := dataforseo.New("synthetic-api-key")
	api.BaseURL = fake.URL
	ctx, session := connectTestClient(t, newServer(toolset.New(seo.New(api))))
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "get_ranked_keywords", Arguments: map[string]any{"target": "example.com"}})
	if err != nil || result == nil || !result.IsError {
		t.Fatalf("provider error = (%+v, %v), want tool error", result, err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %+v", result.Content)
	}
	if got := result.Content[0].(*mcp.TextContent).Text; got != "DataForSEO authentication failed. Check the server credentials." {
		t.Errorf("error text = %q", got)
	}
}

func TestToolError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"input", fmt.Errorf("wrapped: %w", &seo.InputError{Msg: "target is required"}), "target is required"},
		{"upstream HTTP", &dataforseo.Error{HTTPStatus: 503, Message: "private body"}, "DataForSEO upstream service failed. Try again later."},
		{"upstream task", &dataforseo.Error{StatusCode: 50000, Message: "private body"}, "DataForSEO upstream service failed. Try again later."},
		{"rate limited", &dataforseo.Error{HTTPStatus: 429, Message: "private body"}, "DataForSEO rate limited this request. Try again later."},
		{"auth HTTP", &dataforseo.Error{HTTPStatus: 401, Message: "private body"}, "DataForSEO authentication failed. Check the server credentials."},
		{"auth task", fmt.Errorf("wrapped: %w", &dataforseo.Error{StatusCode: 40100, Message: "private body"}), "DataForSEO authentication failed. Check the server credentials."},
		{"task error keeps the status message", &dataforseo.Error{StatusCode: 40501, Message: "Invalid Field: 'target'."}, "DataForSEO request failed: Invalid Field: 'target'."},
		{"HTTP error hides the body", &dataforseo.Error{HTTPStatus: 400, Message: "private body"}, "DataForSEO request failed."},
		{"unexpected", errors.New("private details"), "SEO research failed."},
		{"canceled", context.Canceled, "SEO research canceled."},
		{"deadline", context.DeadlineExceeded, "SEO research timed out."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toolError(tt.err)
			if !got.IsError || got.StructuredContent != nil || len(got.Content) != 1 {
				t.Fatalf("invalid error result: %+v", got)
			}
			if text := got.Content[0].(*mcp.TextContent).Text; text != tt.want {
				t.Errorf("text = %q, want %q", text, tt.want)
			}
		})
	}
}
