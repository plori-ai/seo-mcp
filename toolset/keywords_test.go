package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
)

// newKeywordsSet returns a Set backed by a fake DataForSEO server that answers
// each path with the task result in results.
func newKeywordsSet(t *testing.T, results map[string]string) *Set {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, ok := results[r.URL.Path]
		if !ok {
			t.Errorf("unexpected request to %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"status_code":20000,"status_message":"Ok.","tasks":[{"status_code":20000,"status_message":"Ok.","cost":0.01,"result":` + result + `}]}`))
	}))
	t.Cleanup(srv.Close)
	api := dataforseo.New("dGVzdDp0ZXN0")
	api.BaseURL = srv.URL
	return New(seo.New(api))
}

func keywordsToolByName(t *testing.T, name string) Tool {
	t.Helper()
	for _, tool := range Tools() {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("no tool %q", name)
	return Tool{}
}

// kwJSONKeys returns the sorted keys of a JSON object.
func kwJSONKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("not a JSON object: %s", b)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// kwJSONTags returns the sorted JSON field names of a struct type.
func kwJSONTags(typ reflect.Type) []string {
	var out []string
	for i := range typ.NumField() {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// TestKeywordToolSchemas checks that each schema is valid JSON and names
// exactly the fields the request struct decodes.
func TestKeywordToolSchemas(t *testing.T) {
	tests := []struct {
		tool    string
		request reflect.Type
	}{
		{"research_keywords", reflect.TypeFor[seo.ResearchKeywordsRequest]()},
		{"get_keyword_metrics", reflect.TypeFor[seo.KeywordMetricsRequest]()},
	}
	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			tool := keywordsToolByName(t, tc.tool)
			var schema struct {
				Type       string                     `json:"type"`
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Fatalf("schema is not valid JSON: %v", err)
			}
			if schema.Type != "object" {
				t.Errorf("schema type = %q, want object", schema.Type)
			}
			if got := kwJSONKeys(t, schema.Properties); !slices.Equal(got, kwJSONTags(tc.request)) {
				t.Errorf("schema properties = %v, request fields = %v", got, kwJSONTags(tc.request))
			}
			if tool.Title == "" || tool.Description == "" {
				t.Error("title or description is empty")
			}
		})
	}

	var seeds struct {
		Properties struct {
			Seeds struct {
				Items struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"items"`
			} `json:"seeds"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(keywordsToolByName(t, "research_keywords").InputSchema, &seeds); err != nil {
		t.Fatal(err)
	}
	if got, want := kwJSONKeys(t, seeds.Properties.Seeds.Items.Properties), kwJSONTags(reflect.TypeFor[seo.KeywordSeed]()); !slices.Equal(got, want) {
		t.Errorf("seed item properties = %v, KeywordSeed fields = %v", got, want)
	}
}

const keywordsRelatedResult = `[{"items":[
	{"keyword_data":{"keyword":"coffee grinder","keyword_info":{"search_volume":5000,"cpc":1.2,"competition":0.8,"monthly_searches":[{"year":2026,"month":8,"search_volume":5200}]},"keyword_properties":{"keyword_difficulty":42},"search_intent_info":{"main_intent":"commercial"}}},
	{"keyword_data":{"keyword":"burr grinder","keyword_info":{"search_volume":1000,"cpc":null,"competition":0.5}}},
	{"keyword_data":{"keyword":"manual grinder","keyword_info":{"search_volume":300}}},
	{"keyword_data":{"keyword":"electric grinder","keyword_info":{"search_volume":200}}},
	{"keyword_data":{"keyword":"grinder reviews","keyword_info":{"search_volume":100}}},
	{"keyword_data":{"keyword":"best grinder","keyword_info":{"search_volume":90}}}
]}]`

const keywordsOverviewResult = `[{"items":[
	{"keyword":"coffee grinder","keyword_info":{"search_volume":5000,"cpc":1.2,"competition":0.8,"competition_level":"HIGH","monthly_searches":[{"year":2026,"month":8,"search_volume":5200}]},"keyword_properties":{"keyword_difficulty":42},"search_intent_info":{"main_intent":"commercial"}},
	{"keyword":"burr grinder","keyword_info":{"search_volume":1000,"cpc":1.5,"competition":0.5,"competition_level":"MEDIUM","monthly_searches":[]},"keyword_properties":{"keyword_difficulty":null},"search_intent_info":{"main_intent":null}}
]}]`

// TestKeywordToolsCall sends arguments in the exact form an OpenSEO client
// sends them and checks the field names of the result.
func TestKeywordToolsCall(t *testing.T) {
	set := newKeywordsSet(t, map[string]string{
		"/v3/dataforseo_labs/google/related_keywords/live": keywordsRelatedResult,
		"/v3/dataforseo_labs/google/keyword_overview/live": keywordsOverviewResult,
	})
	ctx := context.Background()

	res, err := set.Call(ctx, "research_keywords", json.RawMessage(
		`{"seeds":[{"seed":"coffee grinder","locationCode":2840,"languageCode":"en"}],"resultLimit":150,"includeClickstreamData":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := kwJSONKeys(t, res); !slices.Equal(got, []string{"results"}) {
		t.Errorf("research_keywords keys = %v", got)
	}
	research := res.(*seo.ResearchKeywordsResult)
	if len(research.Results) != 1 || !research.Results[0].OK || research.Results[0].RowCount != 6 {
		t.Fatalf("research_keywords result = %+v", research.Results)
	}
	if got, want := kwJSONKeys(t, research.Results[0]), []string{"ok", "rowCount", "rows", "seed", "source", "usedFallback"}; !slices.Equal(got, want) {
		t.Errorf("seed result keys = %v, want %v", got, want)
	}
	if got, want := kwJSONKeys(t, research.Results[0].Rows[0]), []string{"competition", "cpc", "intent", "keyword", "keywordDifficulty", "searchVolume", "trend"}; !slices.Equal(got, want) {
		t.Errorf("row keys = %v, want %v", got, want)
	}

	res, err = set.Call(ctx, "get_keyword_metrics", json.RawMessage(
		`{"keywords":["burr grinder","coffee grinder","unknown keyword"],"locationCode":2840,"languageCode":"en","includeMonthlyTrends":true,"sortBy":"cpc","includeClickstreamData":false}`))
	if err != nil {
		t.Fatal(err)
	}
	metrics := res.(*seo.KeywordMetricsResult)
	if len(metrics.Keywords) != 2 || metrics.Keywords[0].Keyword != "burr grinder" {
		t.Fatalf("get_keyword_metrics rows = %+v, want 2 rows sorted by cpc", metrics.Keywords)
	}
	if got, want := kwJSONKeys(t, metrics.Keywords[0]), []string{"competition", "competition_level", "cpc", "keyword", "keyword_difficulty", "main_intent", "monthly_searches", "search_volume"}; !slices.Equal(got, want) {
		t.Errorf("metrics row keys = %v, want %v", got, want)
	}
}

func TestKeywordToolsInvalidArguments(t *testing.T) {
	set := newKeywordsSet(t, nil)
	for name, args := range map[string]string{
		"research_keywords":   `{"seeds":"coffee"}`,
		"get_keyword_metrics": `{"keywords":["a"],"sortBy":"rank"}`,
	} {
		_, err := set.Call(context.Background(), name, json.RawMessage(args))
		var ie *seo.InputError
		if !errors.As(err, &ie) {
			t.Errorf("%s(%s): err = %v, want *seo.InputError", name, args, err)
		}
	}
}
