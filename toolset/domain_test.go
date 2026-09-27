package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
)

func domainToolSchema(t *testing.T, tool Tool, required string) map[string]map[string]any {
	t.Helper()
	var schema struct {
		Type       string                    `json:"type"`
		Properties map[string]map[string]any `json:"properties"`
		Required   []string                  `json:"required"`
	}
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Type != "object" || !reflect.DeepEqual(schema.Required, []string{required}) {
		t.Fatalf("schema = %s", tool.InputSchema)
	}
	if _, ok := schema.Properties["projectId"]; ok {
		t.Error("schema exposes projectId")
	}
	for _, text := range []string{tool.Description, string(tool.InputSchema)} {
		for _, forbidden := range []string{"project", "credits", "cached", "openseo app"} {
			if strings.Contains(strings.ToLower(text), forbidden) {
				t.Errorf("tool %s contains %q", tool.Name, forbidden)
			}
		}
	}
	if !strings.Contains(tool.Description, "billed by DataForSEO") {
		t.Errorf("tool %s lacks billing description", tool.Name)
	}
	var raw any
	if err := json.Unmarshal(tool.InputSchema, &raw); err != nil {
		t.Fatal(err)
	}
	var checkPatterns func(any)
	checkPatterns = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			if pattern, ok := value["pattern"].(string); ok {
				if _, err := regexp.Compile(pattern); err != nil {
					t.Errorf("MCP SDK cannot compile schema pattern: %v", err)
				}
			}
			for _, child := range value {
				checkPatterns(child)
			}
		case []any:
			for _, child := range value {
				checkPatterns(child)
			}
		}
	}
	checkPatterns(raw)
	return schema.Properties
}

func TestDomainToolSchemas(t *testing.T) {
	for _, tt := range []struct {
		tool     Tool
		required string
	}{{rankedKeywordsTool, "target"}, {domainOverviewTool, "domain"}} {
		t.Run(tt.tool.Name, func(t *testing.T) {
			props := domainToolSchema(t, tt.tool, tt.required)
			if props[tt.required]["maxLength"] != float64(2048) {
				t.Errorf("target constraints = %v", props[tt.required])
			}
			if props["locationCode"]["type"] != "integer" || props["locationCode"]["minimum"] != float64(1) {
				t.Error("locationCode constraints missing")
			}
			if props["includeSubdomains"]["deprecated"] != true {
				t.Error("legacy includeSubdomains missing")
			}
			wantScopes := []any{"exact_url", "subfolder", "domain", "subdomains"}
			if !reflect.DeepEqual(props["scope"]["enum"], wantScopes) {
				t.Errorf("scope = %v", props["scope"])
			}
			if tt.tool.Name == "get_ranked_keywords" {
				for _, rule := range []struct {
					field, key string
					value      float64
				}{{"limit", "minimum", 1}, {"limit", "maximum", 100}, {"offset", "maximum", 1000}, {"maxRank", "maximum", 100}, {"resultTypes", "maxItems", 5}, {"excludeBrandTerms", "maxItems", 10}} {
					if props[rule.field][rule.key] != rule.value {
						t.Errorf("%s %s = %v", rule.field, rule.key, props[rule.field][rule.key])
					}
				}
			}
		})
	}
}

func TestDomainToolsCall(t *testing.T) {
	for _, tt := range []struct{ name, args, path string }{{"get_ranked_keywords", `{"target":"example.com","locationCode":2826,"languageCode":"en","limit":3}`, "ranked_keywords"}, {"get_domain_overview", `{"domain":"example.com","scope":"domain","locationCode":2826,"languageCode":"en"}`, "domain_rank_overview"}} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v3/dataforseo_labs/google/"+tt.path+"/live" {
					t.Errorf("path = %s", r.URL.Path)
				}
				var tasks []map[string]any
				if err := json.NewDecoder(r.Body).Decode(&tasks); err != nil {
					t.Error(err)
				}
				if len(tasks) != 1 || tasks[0]["location_code"] != float64(2826) || tasks[0]["language_code"] != "en" {
					t.Errorf("tasks = %v", tasks)
				}
				_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"result":[{"items":[]}]}]}`))
			}))
			defer server.Close()
			api := dataforseo.New("synthetic-test-key")
			api.BaseURL = server.URL
			result, err := New(seo.New(api)).Call(context.Background(), tt.name, json.RawMessage(tt.args))
			if err != nil {
				t.Fatal(err)
			}
			switch tt.name {
			case "get_ranked_keywords":
				if _, ok := result.(*seo.RankedKeywordsResult); !ok {
					t.Fatalf("result type %T", result)
				}
			case "get_domain_overview":
				if _, ok := result.(*seo.DomainOverviewResult); !ok {
					t.Fatalf("result type %T", result)
				}
			}
		})
	}
}

func TestDomainToolArgumentErrors(t *testing.T) {
	for _, tt := range []struct{ name, args string }{{"get_ranked_keywords", `{"target":"example.com","limit":1.5}`}, {"get_ranked_keywords", `{"target":"example.com","resultTypes":[]}`}, {"get_domain_overview", `{"domain":"example.por"}`}, {"get_domain_overview", `{"domain":42}`}} {
		_, err := New(seo.New(nil)).Call(context.Background(), tt.name, json.RawMessage(tt.args))
		var inputErr *seo.InputError
		if !errors.As(err, &inputErr) {
			t.Errorf("%s %s: %v", tt.name, tt.args, err)
		}
	}
}
