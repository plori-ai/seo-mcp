package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
)

func TestBacklinksToolSchema(t *testing.T) {
	props := domainToolSchema(t, backlinksOverviewTool, "target")
	if !reflect.DeepEqual(props["scope"]["enum"], []any{"exact_url", "subfolder", "domain", "subdomains", "page"}) {
		t.Errorf("scope = %v", props["scope"])
	}
	if props["hideSpam"]["type"] != "boolean" {
		t.Errorf("hideSpam = %v", props["hideSpam"])
	}
}

func TestBacklinksToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tasks []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&tasks); err != nil {
			t.Error(err)
		}
		if len(tasks) != 1 || tasks[0]["target"] != "https://example.com/" || tasks[0]["include_subdomains"] != true {
			t.Errorf("tasks = %v", tasks)
		}
		if len(tasks) == 1 {
			if _, ok := tasks[0]["filters"]; ok {
				t.Error("hideSpam=false sent spam filter")
			}
		}
		if r.URL.Path != "/v3/backlinks/summary/live" && r.URL.Path != "/v3/backlinks/referring_domains/live" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"result":null}]}`))
	}))
	defer server.Close()
	api := dataforseo.New("synthetic-test-key")
	api.BaseURL = server.URL
	result, err := New(seo.New(api)).Call(context.Background(), "get_backlinks_overview", json.RawMessage(`{"target":"example.com","scope":"page","hideSpam":false}`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := result.(*seo.BacklinksOverviewResult)
	if !ok {
		t.Fatalf("result type %T", result)
	}
	if got.Target != "https://example.com/" || got.Scope != "exact_url" || got.ReferringDomains == nil {
		t.Fatalf("result = %+v", got)
	}
}

func TestBacklinksToolArgumentErrors(t *testing.T) {
	for _, args := range []string{`{}`, `{"target":"example.com","hideSpam":"false"}`, `{"target":"example.com","scope":"bad"}`} {
		_, err := New(seo.New(nil)).Call(context.Background(), "get_backlinks_overview", json.RawMessage(args))
		var inputErr *seo.InputError
		if !errors.As(err, &inputErr) {
			t.Errorf("%s: %v", args, err)
		}
	}
}
