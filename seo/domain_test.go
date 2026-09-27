package seo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

func domainTestPtr[T any](value T) *T { return &value }

func domainTestJSON(t *testing.T, got any, want string) {
	t.Helper()
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var actual, expected any
	if err := json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("JSON mismatch\ngot  %s\nwant %s", raw, want)
	}
}

func domainTestBody(t *testing.T, r *http.Request, want string) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Errorf("method = %s", r.Method)
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		t.Error(err)
		return
	}
	domainTestJSON(t, json.RawMessage(raw), "["+want+"]")
}

func domainTestClient(t *testing.T, handler http.HandlerFunc, opts ...Option) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	api := dataforseo.New("synthetic-test-key")
	api.BaseURL = server.URL
	api.HTTPClient = server.Client()
	return New(api, opts...)
}

func domainTestResponse(w http.ResponseWriter, status int, message, result string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status_code": 20000, "tasks": []any{map[string]any{"status_code": status, "status_message": message, "result": json.RawMessage(result)}}})
}

func domainTestFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestRankedKeywordsRequests(t *testing.T) {
	fixture := domainTestFixture(t, "testdata/domain/ranked.json")
	for _, tt := range []struct {
		name, args, task, target, scope string
		opts                            []Option
	}{
		{"default", `{"target":"example.com"}`, `{"target":"example.com","location_code":2840,"language_code":"en","limit":50,"order_by":["keyword_data.keyword_info.search_volume,desc"]}`, "example.com", "subdomains", nil},
		{"client market", `{"target":"example.com"}`, `{"target":"example.com","location_code":2276,"language_code":"de","limit":50,"order_by":["keyword_data.keyword_info.search_volume,desc"]}`, "example.com", "subdomains", []Option{WithDefaultMarket(Market{2276, "de"})}},
		{"explicit market and domain", `{"target":"example.com","scope":"domain","locationCode":2826,"languageCode":"en","resultTypes":["organic","featured_snippet"],"sortBy":"rank","limit":100,"offset":0}`, `{"target":"example.com","location_code":2826,"language_code":"en","limit":100,"offset":0,"item_types":["organic","featured_snippet"],"order_by":["ranked_serp_element.serp_item.rank_absolute,asc"],"filters":[["ranked_serp_element.serp_item.domain","in",["example.com","www.example.com"]]]}`, "example.com", "domain", nil},
		{"scoped filters", `{"target":"https://www.example.com/a_b%20c/?q=1#top","scope":"exact_url","minSearchVolume":0,"maxRank":7,"excludeBrandTerms":["A_%"],"sortBy":"cpc","limit":1,"offset":1000}`, `{"target":"example.com","location_code":2840,"language_code":"en","limit":1,"offset":1000,"order_by":["keyword_data.keyword_info.cpc,desc"],"filters":[["ranked_serp_element.serp_item.domain","in",["example.com","www.example.com"]],"and",[["ranked_serp_element.serp_item.relative_url","in",["/a_b%20c","/a_b%20c/"]],"or",["ranked_serp_element.serp_item.relative_url","like","/a\\_b\\%20c?%"],"or",["ranked_serp_element.serp_item.relative_url","like","/a\\_b\\%20c/?%"]],"and",["keyword_data.keyword_info.search_volume",">=",0],"and",["ranked_serp_element.serp_item.rank_absolute","<=",7],"and",["keyword_data.keyword","not_ilike","%A_%%"]]}`, "example.com/a_b%20c", "exact_url", nil},
		{"subfolder", `{"target":"https://example.com/Blog/","sortBy":"traffic_estimate"}`, `{"target":"example.com","location_code":2840,"language_code":"en","limit":50,"order_by":["ranked_serp_element.serp_item.etv,desc"],"filters":[["ranked_serp_element.serp_item.domain","in",["example.com","www.example.com"]],"and",[["ranked_serp_element.serp_item.relative_url","=","/Blog"],"or",["ranked_serp_element.serp_item.relative_url","like","/Blog/%"],"or",["ranked_serp_element.serp_item.relative_url","like","/Blog?%"]]]}`, "example.com/Blog", "subfolder", nil},
		{"legacy market", `{"target":"example.com","market":{"country":"USA"}}`, `{"target":"example.com","location_code":2840,"language_code":"en","limit":50,"order_by":["keyword_data.keyword_info.search_volume,desc"]}`, "example.com", "subdomains", []Option{WithDefaultMarket(Market{2276, "de"})}},
		{"market precedence", `{"target":"example.com","market":{"country":"USA"},"locationCode":2276}`, `{"target":"example.com","location_code":2276,"language_code":"de","limit":50,"order_by":["keyword_data.keyword_info.search_volume,desc"]}`, "example.com", "subdomains", nil},
		{"legacy URL scope", `{"target":"https://example.com/page/","includeSubdomains":true}`, `{"target":"example.com","location_code":2840,"language_code":"en","limit":50,"order_by":["keyword_data.keyword_info.search_volume,desc"],"filters":[["ranked_serp_element.serp_item.domain","in",["example.com","www.example.com"]],"and",[["ranked_serp_element.serp_item.relative_url","in",["/page","/page/"]],"or",["ranked_serp_element.serp_item.relative_url","like","/page?%"],"or",["ranked_serp_element.serp_item.relative_url","like","/page/?%"]]]}`, "example.com/page", "exact_url", nil},
		{"scope precedence", `{"target":"example.com","scope":"subdomains","includeSubdomains":false}`, `{"target":"example.com","location_code":2840,"language_code":"en","limit":50,"order_by":["keyword_data.keyword_info.search_volume,desc"]}`, "example.com", "subdomains", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v3/dataforseo_labs/google/ranked_keywords/live" {
					t.Errorf("path = %s", r.URL.Path)
				}
				domainTestBody(t, r, tt.task)
				domainTestResponse(w, 20000, "Ok.", fixture)
			}, tt.opts...)
			var req RankedKeywordsRequest
			if err := json.Unmarshal([]byte(tt.args), &req); err != nil {
				t.Fatal(err)
			}
			got, err := c.RankedKeywords(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 || got.Target != tt.target || got.Scope != tt.scope {
				t.Fatalf("calls=%d result=%+v", calls.Load(), got)
			}
			domainTestJSON(t, got.Keywords, `[{"keyword_data":{"keyword":"sample query","keyword_info":{"search_volume":12,"cpc":0.25},"provider_extra":null},"ranked_serp_element":{"serp_item":{"rank_absolute":3,"url":"https://example.com/page"}},"new_provider_field":{"values":[1,true,null]}}]`)
			if got.TotalCount == nil || *got.TotalCount != 9 {
				t.Fatalf("total = %v", got.TotalCount)
			}
		})
	}
}

func TestDomainOverviewRequestsAndShape(t *testing.T) {
	for _, tt := range []struct {
		name                             string
		req                              DomainOverviewRequest
		location                         int
		language, result, scope, display string
		traffic, count                   *float64
		hasData                          bool
	}{
		{"default", DomainOverviewRequest{Domain: "https://www.Example.com/blog?x=1"}, 2840, "en", `[{"items":[{"metrics":{"organic":{"etv":10.5,"count":2.4}}}]}]`, "subfolder", "example.com/blog", domainTestPtr(11.0), domainTestPtr(2.0), true},
		{"explicit market", DomainOverviewRequest{Domain: "example.com", Scope: "domain", LocationCode: 2826, LanguageCode: "en"}, 2826, "en", `[{"items":[{"metrics":{"organic":{"etv":0,"count":0}}}]}]`, "domain", "example.com", domainTestPtr(0.0), domainTestPtr(0.0), false},
		{"missing metrics", DomainOverviewRequest{Domain: "example.com", IncludeSubdomains: domainTestPtr(false)}, 2840, "en", `[{"items":[{}]}]`, "domain", "example.com", nil, nil, false},
		{"null metrics", DomainOverviewRequest{Domain: "example.com"}, 2840, "en", `[{"items":[{"metrics":null}]}]`, "subdomains", "example.com", nil, nil, false},
		{"empty items", DomainOverviewRequest{Domain: "example.com"}, 2840, "en", `[{"items":[]}]`, "subdomains", "example.com", nil, nil, false},
		{"null result", DomainOverviewRequest{Domain: "example.com"}, 2840, "en", `null`, "subdomains", "example.com", nil, nil, false},
		{"scope overrides legacy", DomainOverviewRequest{Domain: "example.com/page", Scope: "exact_url", IncludeSubdomains: domainTestPtr(false)}, 2840, "en", `[]`, "exact_url", "example.com/page", nil, nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload, _ := json.Marshal(map[string]any{"target": "example.com", "location_code": tt.location, "language_code": tt.language, "limit": 1})
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v3/dataforseo_labs/google/domain_rank_overview/live" {
					t.Errorf("path = %s", r.URL.Path)
				}
				domainTestBody(t, r, string(payload))
				domainTestResponse(w, 20000, "Ok.", tt.result)
			})
			got, err := c.DomainOverview(context.Background(), tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := time.Parse("2006-01-02T15:04:05.000Z", got.FetchedAt); err != nil {
				t.Fatal(err)
			}
			want := DomainOverviewResult{Domain: "example.com", Scope: tt.scope, DisplayTarget: tt.display, OrganicTraffic: tt.traffic, OrganicKeywords: tt.count, HasData: tt.hasData, FetchedAt: got.FetchedAt}
			if !reflect.DeepEqual(got, &want) {
				t.Fatalf("got %+v, want %+v", got, want)
			}
			raw, _ := json.Marshal(got)
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(raw, &fields)
			if len(fields) != 9 || string(fields["backlinks"]) != "null" || string(fields["referringDomains"]) != "null" {
				t.Fatalf("shape = %s", raw)
			}
		})
	}
}

func TestDomainInputValidation(t *testing.T) {
	for _, args := range []string{
		`{}`, `{"target":"www.example.com"}`, `{"target":"example.com/path"}`, `{"target":"example.por"}`, `{"target":"example.com","scope":"page"}`, `{"target":"example.com","scope":"subfolder"}`,
		`{"target":"example.com","limit":0}`, `{"target":"example.com","limit":101}`, `{"target":"example.com","offset":-1}`, `{"target":"example.com","offset":1001}`,
		`{"target":"example.com","minSearchVolume":-1}`, `{"target":"example.com","maxRank":0}`, `{"target":"example.com","maxRank":101}`, `{"target":"example.com","sortBy":"bad"}`,
		`{"target":"example.com","resultTypes":[]}`, `{"target":"example.com","resultTypes":["bad"]}`, `{"target":"example.com","excludeBrandTerms":[]}`, `{"target":"example.com","excludeBrandTerms":[""]}`,
		`{"target":"example.com","market":{"country":"GB"}}`, `{"target":"example.com","locationCode":9999}`, `{"target":"example.com","locationCode":2352}`, `{"target":"example.com","languageCode":"bad"}`,
		`{"target":"https://example.com/page","minSearchVolume":0,"maxRank":5,"excludeBrandTerms":["a","b","c"]}`,
	} {
		t.Run(args, func(t *testing.T) {
			var req RankedKeywordsRequest
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				t.Fatal(err)
			}
			_, err := New(nil).RankedKeywords(context.Background(), req)
			var inputErr *InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("want InputError, got %v", err)
			}
		})
	}
	for _, req := range []DomainOverviewRequest{{}, {Domain: "bad.por"}, {Domain: strings.Repeat("x", 2049)}, {Domain: "example.com", LocationCode: 2352}, {Domain: "example.com", LanguageCode: "bad"}} {
		_, err := New(nil).DomainOverview(context.Background(), req)
		var inputErr *InputError
		if !errors.As(err, &inputErr) {
			t.Errorf("request %+v: want InputError, got %v", req, err)
		}
	}
}

func TestDomainEmptyAndTaskErrors(t *testing.T) {
	for _, tt := range []struct {
		name            string
		status          int
		message, result string
	}{
		{"null items", 20000, "Ok.", `[{"items":null}]`}, {"no result", 20000, "Ok.", `[]`},
		{"no search results", 40501, "No Search Results.", `null`}, {"invalid target", 40501, "Invalid Field: 'target'.", `null`}, {"provider error", 40101, "Internal SE Server Error.", `null`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) { domainTestResponse(w, tt.status, tt.message, tt.result) })
			ranked, err := c.RankedKeywords(context.Background(), RankedKeywordsRequest{Target: "example.com"})
			if tt.status == 20000 {
				if err != nil {
					t.Fatal(err)
				}
				domainTestJSON(t, ranked, `{"keywords":[],"totalCount":null,"target":"example.com","scope":"subdomains"}`)
			} else {
				var apiErr *dataforseo.Error
				if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
					t.Fatalf("error = %v", err)
				}
			}
			_, err = c.DomainOverview(context.Background(), DomainOverviewRequest{Domain: "example.com"})
			if tt.status != 20000 {
				var apiErr *dataforseo.Error
				if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
					t.Fatalf("overview error = %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
