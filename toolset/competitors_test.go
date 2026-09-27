package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"testing"

	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
)

func TestSerpCompetitorsToolSchema(t *testing.T) {
	props := domainToolSchema(t, findSerpCompetitorsTool, "keywords")
	for _, rule := range []struct {
		field, key string
		value      float64
	}{
		{"keywords", "minItems", 1}, {"keywords", "maxItems", 100},
		{"resultTypes", "minItems", 1}, {"resultTypes", "maxItems", 4},
		{"excludeDomains", "minItems", 1}, {"excludeDomains", "maxItems", 50},
		{"locationCode", "minimum", 1}, {"limit", "minimum", 1}, {"limit", "maximum", 100},
		{"offset", "minimum", 0}, {"offset", "maximum", 1000},
	} {
		if props[rule.field][rule.key] != rule.value {
			t.Errorf("%s %s = %v", rule.field, rule.key, props[rule.field][rule.key])
		}
	}
	if !reflect.DeepEqual(props["sortBy"]["enum"], []any{"visibility", "traffic_estimate", "avg_position", "keyword_count"}) {
		t.Errorf("sortBy = %v", props["sortBy"])
	}
	for field, length := range map[string]float64{"keywords": 120, "excludeDomains": 255} {
		items, ok := props[field]["items"].(map[string]any)
		if !ok || items["maxLength"] != length || items["minLength"] != float64(1) {
			t.Errorf("%s items = %v", field, props[field]["items"])
		}
	}
	if props["includeSubdomains"]["type"] != "boolean" {
		t.Errorf("includeSubdomains = %v", props["includeSubdomains"])
	}
	var fields []string
	for field := range props {
		fields = append(fields, field)
	}
	slices.Sort(fields)
	if want := kwJSONTags(reflect.TypeFor[seo.FindSerpCompetitorsRequest]()); !reflect.DeepEqual(fields, want) {
		t.Errorf("schema fields=%v request fields=%v", fields, want)
	}
}

func TestSerpCompetitorsToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/dataforseo_labs/google/serp_competitors/live" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var got, want any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal([]byte(`[{"keywords":["plumber"],"location_code":2826,"language_code":"en","item_types":["paid"],"include_subdomains":false,"limit":100,"offset":1}]`), &want); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("request body=%v want=%v", got, want)
		}
		_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"result":[{"items":[{"domain":"skip.example","avg_position":1},{"domain":"second.example","avg_position":5},{"domain":"first.example","avg_position":2,"extra":true}]}]}]}`))
	}))
	defer server.Close()
	api := dataforseo.New("synthetic-test-key")
	api.BaseURL = server.URL
	result, err := New(seo.New(api)).Call(context.Background(), "find_serp_competitors", json.RawMessage(`{"keywords":["plumber"],"locationCode":2826,"languageCode":"en","resultTypes":["paid"],"includeSubdomains":false,"excludeDomains":["skip.example"],"sortBy":"avg_position","limit":100,"offset":1}`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := result.(*seo.FindSerpCompetitorsResult)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"competitors":[{"domain":"first.example","avg_position":2,"extra":true},{"domain":"second.example","avg_position":5}]}` {
		t.Fatalf("result = %s", raw)
	}
}

func TestSerpCompetitorsToolInputErrors(t *testing.T) {
	for _, args := range []string{`{}`, `{"keywords":[]}`, `{"keywords":"plumber"}`, `{"keywords":["a"],"limit":1.5}`, `{"keywords":["a"],"includeSubdomains":"false"}`, `{"keywords":["a"],"resultTypes":["bad"]}`, `{"keywords":["a"],"locationCode":2352}`} {
		_, err := New(seo.New(nil)).Call(context.Background(), "find_serp_competitors", json.RawMessage(args))
		var inputErr *seo.InputError
		if !errors.As(err, &inputErr) {
			t.Errorf("%s: %v", args, err)
		}
	}
}
