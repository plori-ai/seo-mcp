package seo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

// kwReply is what the fake DataForSEO server answers for one task.
type kwReply struct {
	Status  int    // task status_code; 0 means 20000
	Message string // task status_message; empty means "Ok."
	Result  any    // task result
}

// kwCall is one task the fake server received.
type kwCall struct {
	Path string
	Task map[string]any
}

type kwRecorder struct {
	mu    sync.Mutex
	calls []kwCall
}

func (r *kwRecorder) add(c kwCall) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, c)
}

func (r *kwRecorder) all() []kwCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

func (r *kwRecorder) paths() []string {
	var out []string
	for _, c := range r.all() {
		out = append(out, c.Path)
	}
	return out
}

// newKWClient returns a Client whose DataForSEO API is a fake server that
// answers each task with handle.
func newKWClient(t *testing.T, handle func(path string, task map[string]any) kwReply, opts ...Option) (*Client, *kwRecorder) {
	t.Helper()
	rec := &kwRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tasks []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&tasks); err != nil || len(tasks) != 1 {
			t.Errorf("%s: request body is not one task (err %v)", r.URL.Path, err)
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		rec.add(kwCall{Path: r.URL.Path, Task: tasks[0]})
		reply := handle(r.URL.Path, tasks[0])
		status := reply.Status
		if status == 0 {
			status = dataforseo.StatusOK
		}
		msg := reply.Message
		if msg == "" {
			msg = "Ok."
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version":        "0.1.20260801",
			"status_code":    dataforseo.StatusOK,
			"status_message": "Ok.",
			"tasks": []any{map[string]any{
				"id":             "09271200-0000-0000-0000-000000000000",
				"status_code":    status,
				"status_message": msg,
				"cost":           0.01,
				"result_count":   1,
				"path":           strings.Split(strings.Trim(r.URL.Path, "/"), "/"),
				"data":           tasks[0],
				"result":         reply.Result,
			}},
		})
	}))
	t.Cleanup(srv.Close)
	api := dataforseo.New("dGVzdDp0ZXN0")
	api.BaseURL = srv.URL
	return New(api, opts...), rec
}

func kwFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "keywords", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// kwJSON decodes JSON into generic values, so two encodings compare equal
// regardless of field order and spacing.
func kwJSON(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("decode %s: %v", b, err)
	}
	return v
}

func kwAssertJSON(t *testing.T, got any, want []byte) {
	t.Helper()
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(kwJSON(t, b), kwJSON(t, want)) {
		t.Errorf("JSON mismatch\n got: %s\nwant: %s", b, want)
	}
}

func kwAssertTask(t *testing.T, got kwCall, wantPath string, wantTask string) {
	t.Helper()
	if got.Path != wantPath {
		t.Errorf("path = %s, want %s", got.Path, wantPath)
	}
	b, _ := json.Marshal(got.Task)
	if !reflect.DeepEqual(kwJSON(t, b), kwJSON(t, []byte(wantTask))) {
		t.Errorf("task sent to %s\n got: %s\nwant: %s", got.Path, b, wantTask)
	}
}

// labsItem is a minimal DataForSEO Labs keyword item.
func kwLabsItem(keyword string, volume int) map[string]any {
	return map[string]any{
		"keyword":            keyword,
		"keyword_info":       map[string]any{"search_volume": volume, "cpc": 1.0, "competition": 0.5, "monthly_searches": []any{}},
		"keyword_properties": map[string]any{"keyword_difficulty": 10},
		"search_intent_info": map[string]any{"main_intent": "informational"},
	}
}

// kwLabsItems returns labsItem values for the given keywords.
func kwLabsItems(keywords ...string) []map[string]any {
	out := make([]map[string]any, 0, len(keywords))
	for i, k := range keywords {
		out = append(out, kwLabsItem(k, 100-i))
	}
	return out
}

// numbered returns n keywords "<prefix> 1" ... "<prefix> n".
func kwNumbered(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s %d", prefix, i+1)
	}
	return out
}

// kwLabsResult wraps items as keyword_suggestions and keyword_ideas return them.
func kwLabsResult(items []map[string]any) any {
	return []any{map[string]any{"total_count": len(items), "items_count": len(items), "items": items}}
}

// kwRelatedResult wraps items as related_keywords returns them.
func kwRelatedResult(items []map[string]any) any {
	wrapped := make([]any, 0, len(items))
	for _, it := range items {
		wrapped = append(wrapped, map[string]any{"depth": 1, "keyword_data": it})
	}
	return []any{map[string]any{"total_count": len(items), "items_count": len(items), "items": wrapped}}
}

func kwRowKeywords(rows []KeywordRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.Keyword)
	}
	return out
}

func TestResearchKeywordsDefaultMarket(t *testing.T) {
	result := kwFixture(t, "related_keywords_result.json")
	c, rec := newKWClient(t, func(path string, _ map[string]any) kwReply {
		return kwReply{Result: result}
	})
	res, err := c.ResearchKeywords(context.Background(), ResearchKeywordsRequest{
		Seeds:                  []KeywordSeed{{Seed: " Coffee Grinder "}},
		IncludeClickstreamData: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := rec.all()
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want one related_keywords call", rec.paths())
	}
	kwAssertTask(t, calls[0], pathRelatedKeywords, `{
		"keyword": "coffee grinder", "location_code": 2840, "language_code": "en",
		"limit": 150, "depth": 3, "include_clickstream_data": true, "include_serp_info": false}`)
	kwAssertJSON(t, res, kwFixture(t, "research_related_want.json"))
}

func TestResearchKeywordsExplicitMarket(t *testing.T) {
	enough := kwLabsItems(kwNumbered("kw", 5)...)
	c, rec := newKWClient(t, func(path string, _ map[string]any) kwReply {
		return kwReply{Result: kwRelatedResult(enough)}
	}, WithDefaultMarket(Market{LocationCode: 2124, LanguageCode: "fr"}))

	res, err := c.ResearchKeywords(context.Background(), ResearchKeywordsRequest{
		Seeds: []KeywordSeed{
			{Seed: "kaffeemühle", LocationCode: 2276, LanguageCode: "de"},
			{Seed: "zahnbürste", LocationCode: 2276}, // language: Germany's default
			{Seed: "café"},                       // client default market
			{Seed: "coffee", LanguageCode: "en"}, // default location, explicit language
			{Seed: "cafetera", LocationCode: 2840, LanguageCode: "es"},
		},
		ResultLimit: 300,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"kaffeemühle": `{"location_code": 2276, "language_code": "de"}`,
		"zahnbürste":  `{"location_code": 2276, "language_code": "de"}`,
		"café":        `{"location_code": 2124, "language_code": "fr"}`,
		"coffee":      `{"location_code": 2124, "language_code": "en"}`,
		"cafetera":    `{"location_code": 2840, "language_code": "es"}`,
	}
	calls := rec.all()
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %d", rec.paths(), len(want))
	}
	for _, call := range calls {
		kw, _ := call.Task["keyword"].(string)
		market, ok := want[kw]
		if !ok {
			t.Errorf("unexpected keyword %q", kw)
			continue
		}
		var m map[string]any
		_ = json.Unmarshal([]byte(market), &m)
		wantTask := fmt.Sprintf(`{"keyword": %q, "location_code": %v, "language_code": %q,
			"limit": 300, "depth": 3, "include_clickstream_data": false, "include_serp_info": false}`,
			kw, m["location_code"], m["language_code"])
		kwAssertTask(t, call, pathRelatedKeywords, wantTask)
	}
	for i, r := range res.Results {
		if !r.OK || r.Source != sourceRelated || r.UsedFallback || r.RowCount != 5 {
			t.Errorf("result %d = %+v, want 5 related rows", i, r)
		}
	}
	if res.Results[0].Seed != "kaffeemühle" || res.Results[4].Seed != "cafetera" {
		t.Errorf("results are not in request order: %q ... %q", res.Results[0].Seed, res.Results[4].Seed)
	}
}

func TestResearchKeywordsSources(t *testing.T) {
	seed := "grinder"
	many := kwNumbered("idea", 200)
	tests := []struct {
		name         string
		results      map[string][]string // keywords returned per path
		wantPaths    []string
		wantSource   string
		wantFallback bool
		wantRows     []string
	}{
		{
			name:       "related is enough",
			results:    map[string][]string{pathRelatedKeywords: {"grinder", "a", "b", "c", "d", "e"}},
			wantPaths:  []string{pathRelatedKeywords},
			wantSource: sourceRelated,
			wantRows:   []string{"grinder", "a", "b", "c", "d", "e"},
		},
		{
			name: "suggestions fill the gap",
			results: map[string][]string{
				pathRelatedKeywords:    {"grinder", "a", "b"},
				pathKeywordSuggestions: {"grinder", "B", "c", "d", "e"},
			},
			wantPaths:    []string{pathRelatedKeywords, pathKeywordSuggestions},
			wantSource:   sourceSuggestions,
			wantFallback: true,
			wantRows:     []string{"grinder", "a", "b", "c", "d", "e"},
		},
		{
			name: "ideas fill the gap",
			results: map[string][]string{
				pathRelatedKeywords:    {"a"},
				pathKeywordSuggestions: {"b"},
				pathKeywordIdeas:       {"c", "d", "e", "f"},
			},
			wantPaths:    []string{pathRelatedKeywords, pathKeywordSuggestions, pathKeywordIdeas},
			wantSource:   sourceIdeas,
			wantFallback: true,
			wantRows:     []string{"a", "b", "c", "d", "e", "f"},
		},
		{
			name: "all sources short",
			results: map[string][]string{
				pathRelatedKeywords: {"grinder"},
				pathKeywordIdeas:    {"a", "b"},
			},
			wantPaths:    []string{pathRelatedKeywords, pathKeywordSuggestions, pathKeywordIdeas},
			wantSource:   sourceIdeas,
			wantFallback: true,
			wantRows:     []string{"grinder", "a", "b"},
		},
		{
			name: "merged rows stop at the result limit",
			results: map[string][]string{
				pathRelatedKeywords:    {"a", "b"},
				pathKeywordSuggestions: many,
			},
			wantPaths:    []string{pathRelatedKeywords, pathKeywordSuggestions},
			wantSource:   sourceSuggestions,
			wantFallback: true,
			wantRows:     append([]string{"a", "b"}, many[:148]...),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newKWClient(t, func(path string, _ map[string]any) kwReply {
				items := kwLabsItems(tc.results[path]...)
				if path == pathRelatedKeywords {
					return kwReply{Result: kwRelatedResult(items)}
				}
				return kwReply{Result: kwLabsResult(items)}
			})
			res, err := c.ResearchKeywords(context.Background(), ResearchKeywordsRequest{Seeds: []KeywordSeed{{Seed: seed}}})
			if err != nil {
				t.Fatal(err)
			}
			if got := rec.paths(); !slices.Equal(got, tc.wantPaths) {
				t.Fatalf("paths = %v, want %v", got, tc.wantPaths)
			}
			r := res.Results[0]
			if !r.OK || r.Source != tc.wantSource || r.UsedFallback != tc.wantFallback {
				t.Errorf("result = ok %v source %q fallback %v, want source %q fallback %v (error %q)",
					r.OK, r.Source, r.UsedFallback, tc.wantSource, tc.wantFallback, r.Error)
			}
			if got := kwRowKeywords(r.Rows); !slices.Equal(got, tc.wantRows) {
				t.Errorf("rows = %v, want %v", got, tc.wantRows)
			}
			if r.RowCount != len(r.Rows) {
				t.Errorf("rowCount = %d, len(rows) = %d", r.RowCount, len(r.Rows))
			}
			for _, call := range rec.all() {
				switch call.Path {
				case pathKeywordSuggestions:
					kwAssertTask(t, call, pathKeywordSuggestions, `{
						"keyword": "grinder", "location_code": 2840, "language_code": "en", "limit": 150,
						"include_clickstream_data": false, "include_serp_info": false,
						"include_seed_keyword": true, "ignore_synonyms": false, "exact_match": false}`)
				case pathKeywordIdeas:
					kwAssertTask(t, call, pathKeywordIdeas, `{
						"keywords": ["grinder"], "location_code": 2840, "language_code": "en", "limit": 150,
						"include_clickstream_data": false, "include_serp_info": false,
						"ignore_synonyms": false, "closely_variants": false}`)
				}
			}
		})
	}
}

func TestResearchKeywordsGoogleAdsLocation(t *testing.T) {
	items := make([]map[string]any, 0, 160)
	for i := range 160 {
		items = append(items, map[string]any{
			"keyword":           fmt.Sprintf("Kaffi %d", i+1),
			"search_volume":     1000 - i,
			"cpc":               0.5,
			"competition":       "MEDIUM",
			"competition_index": 45,
			"monthly_searches":  []any{map[string]any{"year": 2026, "month": 8, "search_volume": 1000 - i}},
		})
	}
	c, rec := newKWClient(t, func(path string, _ map[string]any) kwReply {
		return kwReply{Result: items}
	})
	res, err := c.ResearchKeywords(context.Background(), ResearchKeywordsRequest{
		Seeds:                  []KeywordSeed{{Seed: "Kaffi", LocationCode: 2352}},
		IncludeClickstreamData: true, // not sent: Google Ads has no clickstream option
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := rec.all()
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want one keywords_for_keywords call", rec.paths())
	}
	kwAssertTask(t, calls[0], pathAdsKeywordsForKeywords, `{
		"keywords": ["kaffi"], "location_code": 2352, "language_code": "is", "sort_by": "search_volume"}`)
	r := res.Results[0]
	if !r.OK || r.Source != sourceGoogleAds || r.UsedFallback || r.RowCount != 150 {
		t.Fatalf("result = ok %v source %q fallback %v rowCount %d (error %q), want 150 google_ads rows",
			r.OK, r.Source, r.UsedFallback, r.RowCount, r.Error)
	}
	kwAssertJSON(t, r.Rows[0], []byte(`{
		"keyword": "kaffi 1", "searchVolume": 1000,
		"trend": [{"year": 2026, "month": 8, "searchVolume": 1000}],
		"cpc": 0.5, "competition": 0.45, "keywordDifficulty": null, "intent": "unknown"}`))
}

func TestResearchKeywordsSeedFailures(t *testing.T) {
	enough := kwLabsItems(kwNumbered("kw", 5)...)
	c, rec := newKWClient(t, func(path string, task map[string]any) kwReply {
		switch task["keyword"] {
		case "no results":
			return kwReply{Status: 40501, Message: "No Search Results."}
		case "flaky":
			if path == pathKeywordSuggestions {
				return kwReply{Status: 40101, Message: "Internal SE Server Error."}
			}
			return kwReply{Result: kwRelatedResult(kwLabsItems("a"))}
		}
		return kwReply{Result: kwRelatedResult(enough)}
	})
	res, err := c.ResearchKeywords(context.Background(), ResearchKeywordsRequest{Seeds: []KeywordSeed{
		{Seed: "good"},
		{Seed: "no results"},
		{Seed: "flaky"},
		{Seed: "wrong language", LanguageCode: "ru"},
		{Seed: "unknown location", LocationCode: 9999},
	}})
	if err != nil {
		t.Fatal(err)
	}
	wantErr := []string{"", "No Search Results", "Internal SE Server Error", "Available: en, es", "location code 9999"}
	for i, r := range res.Results {
		if wantErr[i] == "" {
			if !r.OK {
				t.Errorf("seed %q failed: %s", r.Seed, r.Error)
			}
			continue
		}
		if r.OK || !strings.Contains(r.Error, wantErr[i]) {
			t.Errorf("seed %q = ok %v error %q, want an error containing %q", r.Seed, r.OK, r.Error, wantErr[i])
		}
	}
	// A failed source ends the seed; it does not move on to keyword_ideas.
	for _, call := range rec.all() {
		if kws, _ := call.Task["keywords"].([]any); len(kws) == 1 && kws[0] == "flaky" {
			t.Error("keyword_ideas was called after keyword_suggestions failed")
		}
	}
	// Market errors are found before any request is sent.
	for _, call := range rec.all() {
		if kw := call.Task["keyword"]; kw == "wrong language" || kw == "unknown location" {
			t.Errorf("request sent for invalid market seed %q", kw)
		}
	}
	// A failed seed encodes as {seed, ok, error} only.
	kwAssertJSON(t, res.Results[1], []byte(fmt.Sprintf(`{"seed": "no results", "ok": false, "error": %q}`, res.Results[1].Error)))
}

func TestResearchKeywordsEmptyRowsEncodeAsArray(t *testing.T) {
	c, _ := newKWClient(t, func(path string, _ map[string]any) kwReply {
		return kwReply{Result: []any{map[string]any{"items": nil}}}
	})
	res, err := c.ResearchKeywords(context.Background(), ResearchKeywordsRequest{Seeds: []KeywordSeed{{Seed: "nothing"}}})
	if err != nil {
		t.Fatal(err)
	}
	kwAssertJSON(t, res, []byte(`{"results": [{"seed": "nothing", "ok": true, "rowCount": 0,
		"source": "ideas", "usedFallback": true, "rows": []}]}`))
}

func TestResearchKeywordsInputErrors(t *testing.T) {
	tests := []struct {
		name string
		req  ResearchKeywordsRequest
		want string
	}{
		{"no seeds", ResearchKeywordsRequest{}, "seeds must list 1 to 5"},
		{"six seeds", ResearchKeywordsRequest{Seeds: make([]KeywordSeed, 6)}, "seeds must list 1 to 5"},
		{"blank seed", ResearchKeywordsRequest{Seeds: []KeywordSeed{{Seed: "a"}, {Seed: "  "}}}, "seeds[1].seed is empty"},
		{"bad limit", ResearchKeywordsRequest{Seeds: []KeywordSeed{{Seed: "a"}}, ResultLimit: 200}, "resultLimit must be 150, 300 or 500"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newKWClient(t, func(string, map[string]any) kwReply {
				return kwReply{}
			})
			_, err := c.ResearchKeywords(context.Background(), tc.req)
			var ie *InputError
			if !errors.As(err, &ie) || !strings.Contains(ie.Msg, tc.want) {
				t.Fatalf("err = %v, want *InputError containing %q", err, tc.want)
			}
			if n := len(rec.all()); n != 0 {
				t.Errorf("%d requests sent for invalid input", n)
			}
		})
	}
}

func TestKeywordMetricsLabs(t *testing.T) {
	result := kwFixture(t, "keyword_overview_result.json")
	c, rec := newKWClient(t, func(path string, _ map[string]any) kwReply {
		return kwReply{Result: result}
	})
	res, err := c.KeywordMetrics(context.Background(), KeywordMetricsRequest{
		Keywords: []string{"coffee grinder", "burr grinder", "no data keyword"},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := rec.all()
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want one keyword_overview call", rec.paths())
	}
	kwAssertTask(t, calls[0], pathKeywordOverview, `{
		"keywords": ["coffee grinder", "burr grinder", "no data keyword"],
		"location_code": 2840, "language_code": "en", "include_clickstream_data": false}`)
	kwAssertJSON(t, res, kwFixture(t, "keyword_metrics_want.json"))
}

func TestKeywordMetricsGoogleAdsLocation(t *testing.T) {
	result := kwFixture(t, "ads_search_volume_result.json")
	c, rec := newKWClient(t, func(path string, _ map[string]any) kwReply {
		return kwReply{Result: result}
	})
	res, err := c.KeywordMetrics(context.Background(), KeywordMetricsRequest{
		Keywords:               []string{"kaffikvörn", "kaffivél"},
		LocationCode:           2352,
		IncludeClickstreamData: true, // not sent: Google Ads has no clickstream option
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := rec.all()
	if len(calls) != 1 {
		t.Fatalf("calls = %v, want one search_volume call", rec.paths())
	}
	kwAssertTask(t, calls[0], pathAdsSearchVolume, `{
		"keywords": ["kaffikvörn", "kaffivél"], "location_code": 2352, "language_code": "is"}`)
	kwAssertJSON(t, res, kwFixture(t, "ads_search_volume_want.json"))
}

func TestKeywordMetricsSortAndTrends(t *testing.T) {
	item := func(keyword string, volume, kd any, cpc, competition any) map[string]any {
		return map[string]any{
			"keyword":            keyword,
			"keyword_info":       map[string]any{"search_volume": volume, "cpc": cpc, "competition": competition, "monthly_searches": nil},
			"keyword_properties": map[string]any{"keyword_difficulty": kd},
		}
	}
	items := []map[string]any{
		item("a", 100, 70, 0.5, nil),
		item("b", nil, 20, 3.0, 0.9),
		item("c", 900, nil, nil, 0.1),
		item("d", 50, 90, 1.5, 0.4),
	}
	tests := []struct {
		sortBy string
		want   []string
	}{
		{"", []string{"c", "a", "d", "b"}},
		{"search_volume", []string{"c", "a", "d", "b"}},
		{"keyword_difficulty", []string{"d", "a", "b", "c"}},
		{"cpc", []string{"b", "d", "a", "c"}},
		{"competition", []string{"b", "d", "c", "a"}},
	}
	for _, tc := range tests {
		t.Run("sortBy="+tc.sortBy, func(t *testing.T) {
			c, _ := newKWClient(t, func(string, map[string]any) kwReply {
				return kwReply{Result: kwLabsResult(items)}
			})
			res, err := c.KeywordMetrics(context.Background(), KeywordMetricsRequest{
				Keywords: []string{"a", "b", "c", "d"},
				SortBy:   tc.sortBy,
			})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, k := range res.Keywords {
				got = append(got, k.Keyword)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("order = %v, want %v", got, tc.want)
			}
		})
	}

	for _, include := range []*bool{nil, kwPtr(true), kwPtr(false)} {
		name := "includeMonthlyTrends=unset"
		if include != nil {
			name = fmt.Sprintf("includeMonthlyTrends=%v", *include)
		}
		t.Run(name, func(t *testing.T) {
			c, _ := newKWClient(t, func(string, map[string]any) kwReply {
				return kwReply{Result: kwLabsResult(items[:1])}
			})
			res, err := c.KeywordMetrics(context.Background(), KeywordMetricsRequest{
				Keywords:             []string{"a"},
				IncludeMonthlyTrends: include,
			})
			if err != nil {
				t.Fatal(err)
			}
			b, _ := json.Marshal(res.Keywords[0])
			var row map[string]any
			_ = json.Unmarshal(b, &row)
			v, present := row["monthly_searches"]
			if include != nil && !*include {
				if present {
					t.Errorf("monthly_searches present: %s", b)
				}
			} else if !present || v != nil {
				t.Errorf("monthly_searches = %v (present %v), want null: %s", v, present, b)
			}
		})
	}
}

func TestKeywordMetricsClickstream(t *testing.T) {
	c, rec := newKWClient(t, func(string, map[string]any) kwReply {
		return kwReply{Result: kwLabsResult([]map[string]any{{
			"keyword": "burr grinder",
			"keyword_info": map[string]any{
				"search_volume": 1000, "cpc": 1.5, "competition": 0.5, "competition_level": "MEDIUM",
				"monthly_searches": []any{map[string]any{"year": 2026, "month": 8, "search_volume": 1000}},
			},
			// Unlike research, a zero clickstream volume replaces keyword_info.
			"keyword_info_normalized_with_clickstream": map[string]any{
				"search_volume":    0,
				"monthly_searches": []any{map[string]any{"year": 2026, "month": 8, "search_volume": 0}},
			},
		}})}
	})
	res, err := c.KeywordMetrics(context.Background(), KeywordMetricsRequest{
		Keywords:               []string{"burr grinder"},
		IncludeClickstreamData: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.all()[0].Task["include_clickstream_data"]; got != true {
		t.Errorf("include_clickstream_data = %v, want true", got)
	}
	kwAssertJSON(t, res, []byte(`{"keywords": [{
		"keyword": "burr grinder", "search_volume": 0, "keyword_difficulty": null, "main_intent": null,
		"cpc": 1.5, "competition": 0.5, "competition_level": "MEDIUM",
		"monthly_searches": [{"year": 2026, "month": 8, "search_volume": 0}]}]}`))
}

func TestKeywordMetricsTaskErrors(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		message      string
		wantNoRes    bool
		wantUpstream bool
	}{
		{"no search results", 40501, "No Search Results.", true, false},
		{"upstream failure", 50301, "3rd Party API Service Unavailable.", false, true},
		{"invalid field", 40501, "Invalid Field: 'language_code'.", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newKWClient(t, func(string, map[string]any) kwReply {
				return kwReply{Status: tc.status, Message: tc.message}
			})
			res, err := c.KeywordMetrics(context.Background(), KeywordMetricsRequest{Keywords: []string{"a"}})
			var de *dataforseo.Error
			if !errors.As(err, &de) {
				t.Fatalf("res = %v, err = %v, want a *dataforseo.Error", res, err)
			}
			if de.StatusCode != tc.status || dataforseo.IsNoResults(err) != tc.wantNoRes || de.Upstream() != tc.wantUpstream {
				t.Errorf("err = %v: status %d, no results %v, upstream %v", err, de.StatusCode, dataforseo.IsNoResults(err), de.Upstream())
			}
		})
	}
}

func TestKeywordMetricsInputErrors(t *testing.T) {
	long := strings.Repeat("é", 81)
	tests := []struct {
		name string
		req  KeywordMetricsRequest
		want string
	}{
		{"no keywords", KeywordMetricsRequest{}, "keywords must list 1 to 700"},
		{"too many keywords", KeywordMetricsRequest{Keywords: kwNumbered("k", 701)}, "keywords must list 1 to 700"},
		{"empty keyword", KeywordMetricsRequest{Keywords: []string{"a", ""}}, "keywords[1] must be 1 to 80"},
		{"long keyword", KeywordMetricsRequest{Keywords: []string{long}}, "keywords[0] must be 1 to 80"},
		{"bad sort", KeywordMetricsRequest{Keywords: []string{"a"}, SortBy: "rank"}, "sortBy must be one of"},
		{"language not served", KeywordMetricsRequest{Keywords: []string{"a"}, LanguageCode: "fr"}, "Available: en, es"},
		{"unknown location", KeywordMetricsRequest{Keywords: []string{"a"}, LocationCode: 1}, "location code 1 is not supported"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, rec := newKWClient(t, func(string, map[string]any) kwReply {
				return kwReply{}
			})
			_, err := c.KeywordMetrics(context.Background(), tc.req)
			var ie *InputError
			if !errors.As(err, &ie) || !strings.Contains(ie.Msg, tc.want) {
				t.Fatalf("err = %v, want *InputError containing %q", err, tc.want)
			}
			if n := len(rec.all()); n != 0 {
				t.Errorf("%d requests sent for invalid input", n)
			}
		})
	}
	// 80 characters is the limit, counted in characters rather than bytes.
	c, _ := newKWClient(t, func(string, map[string]any) kwReply {
		return kwReply{Result: kwLabsResult(nil)}
	})
	if _, err := c.KeywordMetrics(context.Background(), KeywordMetricsRequest{Keywords: []string{long[2:]}}); err != nil {
		t.Errorf("80-character keyword rejected: %v", err)
	}
}

func kwPtr[T any](v T) *T { return &v }
