package seo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

func TestSerpCompetitorsRequests(t *testing.T) {
	fixture := domainTestFixture(t, "testdata/competitors/items.json")
	for _, tt := range []struct {
		name, args, task string
		opts             []Option
	}{
		{"default", `{"keywords":["plumber","drain repair"]}`, `{"keywords":["plumber","drain repair"],"location_code":2840,"language_code":"en","item_types":["organic","local_pack"],"limit":50}`, nil},
		{"client market", `{"keywords":["Klempner"]}`, `{"keywords":["Klempner"],"location_code":2276,"language_code":"de","item_types":["organic","local_pack"],"limit":50}`, []Option{WithDefaultMarket(Market{2276, "de"})}},
		{"explicit market and options", `{"keywords":["plumber"],"locationCode":2826,"languageCode":"en","resultTypes":["organic","paid","featured_snippet","local_pack"],"includeSubdomains":false,"sortBy":"traffic_estimate","limit":100,"offset":1000}`, `{"keywords":["plumber"],"location_code":2826,"language_code":"en","item_types":["organic","paid","featured_snippet","local_pack"],"include_subdomains":false,"limit":100,"offset":1000}`, nil},
		{"location primary language", `{"keywords":["Klempner"],"locationCode":2276,"includeSubdomains":true,"limit":1,"offset":0}`, `{"keywords":["Klempner"],"location_code":2276,"language_code":"de","item_types":["organic","local_pack"],"include_subdomains":true,"limit":1,"offset":0}`, nil},
		{"legacy market", `{"keywords":["plumber"],"market":{"country":"USA"}}`, `{"keywords":["plumber"],"location_code":2840,"language_code":"en","item_types":["organic","local_pack"],"limit":50}`, []Option{WithDefaultMarket(Market{2276, "de"})}},
		{"location overrides legacy", `{"keywords":["Klempner"],"market":{"country":"US"},"locationCode":2276}`, `{"keywords":["Klempner"],"location_code":2276,"language_code":"de","item_types":["organic","local_pack"],"limit":50}`, nil},
		{"language overrides legacy", `{"keywords":["Klempner"],"market":{"country":"US"},"languageCode":"de"}`, `{"keywords":["Klempner"],"location_code":2276,"language_code":"de","item_types":["organic","local_pack"],"limit":50}`, []Option{WithDefaultMarket(Market{2276, "de"})}},
		{"empty market keeps default", `{"keywords":["Klempner"],"market":{}}`, `{"keywords":["Klempner"],"location_code":2276,"language_code":"de","item_types":["organic","local_pack"],"limit":50}`, []Option{WithDefaultMarket(Market{2276, "de"})}},
		{"keywords passed unchanged", `{"keywords":[" Plumber "," Plumber "," "]}`, `{"keywords":[" Plumber "," Plumber "," "],"location_code":2840,"language_code":"en","item_types":["organic","local_pack"],"limit":50}`, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v3/dataforseo_labs/google/serp_competitors/live" {
					t.Errorf("path = %s", r.URL.Path)
				}
				domainTestBody(t, r, tt.task)
				domainTestResponse(w, 20000, "Ok.", fixture)
			}, tt.opts...)
			var req FindSerpCompetitorsRequest
			if err := json.Unmarshal([]byte(tt.args), &req); err != nil {
				t.Fatal(err)
			}
			got, err := c.FindSerpCompetitors(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 || len(got.Competitors) != 6 {
				t.Fatalf("calls=%d result=%+v", calls.Load(), got)
			}
		})
	}
}

func TestSerpCompetitorsSortingAndShape(t *testing.T) {
	fixture := domainTestFixture(t, "testdata/competitors/items.json")
	var provider []struct {
		Items []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal([]byte(fixture), &provider); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		sortBy string
		order  []int
	}{
		{"", []int{1, 2, 0, 3, 4, 5}},
		{"visibility", []int{1, 2, 0, 3, 4, 5}},
		{"traffic_estimate", []int{2, 0, 1, 3, 4, 5}},
		{"avg_position", []int{3, 4, 5, 1, 2, 0}},
		{"keyword_count", []int{1, 0, 2, 3, 4, 5}},
	} {
		t.Run(tt.sortBy, func(t *testing.T) {
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				domainTestBody(t, r, `{"keywords":["plumber"],"location_code":2840,"language_code":"en","item_types":["organic","local_pack"],"limit":50}`)
				domainTestResponse(w, 20000, "Ok.", fixture)
			})
			got, err := c.FindSerpCompetitors(context.Background(), FindSerpCompetitorsRequest{Keywords: []string{"plumber"}, SortBy: tt.sortBy})
			if err != nil {
				t.Fatal(err)
			}
			want := make([]json.RawMessage, 0, len(tt.order))
			for _, index := range tt.order {
				want = append(want, provider[0].Items[index])
			}
			raw, err := json.Marshal(map[string]any{"competitors": want})
			if err != nil {
				t.Fatal(err)
			}
			domainTestJSON(t, got, string(raw))
		})
	}
}

func TestSerpCompetitorsExclusions(t *testing.T) {
	fixture := domainTestFixture(t, "testdata/competitors/exclusions.json")
	var calls atomic.Int32
	c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		domainTestBody(t, r, `{"keywords":["plumber"],"location_code":2840,"language_code":"en","item_types":["organic","local_pack"],"limit":8}`)
		domainTestResponse(w, 20000, "Ok.", fixture)
	})
	got, err := c.FindSerpCompetitors(context.Background(), FindSerpCompetitorsRequest{
		Keywords: []string{"plumber"}, ExcludeDomains: []string{"Example.COM", "second.net"}, Limit: domainTestPtr(8),
	})
	if err != nil {
		t.Fatal(err)
	}
	domainTestJSON(t, got, `{"competitors":[{"domain":"notexample.com","visibility":70},{"domain":"example.com.evil.net","visibility":60},{"visibility":10,"provider_extra":"missing domain"},{"domain":42,"visibility":5}]}`)
	if calls.Load() != 1 {
		t.Errorf("exclusions caused %d requests", calls.Load())
	}
}

func TestSerpCompetitorsInputValidation(t *testing.T) {
	for _, args := range []string{
		`{}`, `{"keywords":[]}`, `{"keywords":[""]}`,
		`{"keywords":["a"],"resultTypes":[]}`, `{"keywords":["a"],"resultTypes":["ai_overview_reference"]}`,
		`{"keywords":["a"],"excludeDomains":[]}`, `{"keywords":["a"],"excludeDomains":["www.example.com"]}`,
		`{"keywords":["a"],"excludeDomains":["WWW.example.com"]}`, `{"keywords":["a"],"excludeDomains":["https://example.com"]}`,
		`{"keywords":["a"],"excludeDomains":["example.com/path"]}`, `{"keywords":["a"],"excludeDomains":[""]}`,
		`{"keywords":["a"],"sortBy":"rank"}`, `{"keywords":["a"],"limit":0}`, `{"keywords":["a"],"limit":101}`,
		`{"keywords":["a"],"offset":-1}`, `{"keywords":["a"],"offset":1001}`,
		`{"keywords":["a"],"market":{"country":"GB"}}`, `{"keywords":["a"],"locationCode":-1}`,
		`{"keywords":["a"],"locationCode":9999}`, `{"keywords":["a"],"locationCode":2352}`,
		`{"keywords":["a"],"languageCode":"bad"}`, `{"keywords":["a"],"locationCode":2276,"languageCode":"es"}`,
	} {
		t.Run(args, func(t *testing.T) {
			var req FindSerpCompetitorsRequest
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				t.Fatal(err)
			}
			_, err := New(nil).FindSerpCompetitors(context.Background(), req)
			var inputErr *InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("want InputError, got %v", err)
			}
		})
	}
	for _, req := range []FindSerpCompetitorsRequest{
		{Keywords: make([]string, 101)},
		{Keywords: []string{strings.Repeat("a", 121)}},
		{Keywords: []string{strings.Repeat("😀", 61)}},
		{Keywords: []string{"a"}, ResultTypes: []string{"organic", "organic", "organic", "organic", "organic"}},
		{Keywords: []string{"a"}, ExcludeDomains: make([]string, 51)},
		{Keywords: []string{"a"}, ExcludeDomains: []string{strings.Repeat("a.", 127) + "com"}},
	} {
		_, err := New(nil).FindSerpCompetitors(context.Background(), req)
		var inputErr *InputError
		if !errors.As(err, &inputErr) {
			t.Errorf("request %+v: want InputError, got %v", req, err)
		}
	}
	_, err := New(nil, WithDefaultMarket(Market{2352, "is"})).FindSerpCompetitors(context.Background(), FindSerpCompetitorsRequest{Keywords: []string{"a"}})
	var inputErr *InputError
	if !errors.As(err, &inputErr) {
		t.Errorf("Ads-only default market: %v", err)
	}
}

func TestSerpCompetitorsMaximumInputs(t *testing.T) {
	keywords, exclusions := make([]string, 100), make([]string, 50)
	for i := range keywords {
		keywords[i] = strings.Repeat("😀", 60)
	}
	for i := range exclusions {
		exclusions[i] = "exclude.example"
	}
	want, err := json.Marshal(map[string]any{"keywords": keywords, "location_code": 2840, "language_code": "en", "item_types": []string{"organic", "local_pack"}, "limit": 100})
	if err != nil {
		t.Fatal(err)
	}
	c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		domainTestBody(t, r, string(want))
		domainTestResponse(w, 20000, "Ok.", `[]`)
	})
	got, err := c.FindSerpCompetitors(context.Background(), FindSerpCompetitorsRequest{Keywords: keywords, ExcludeDomains: exclusions, Limit: domainTestPtr(100)})
	if err != nil {
		t.Fatal(err)
	}
	domainTestJSON(t, got, `{"competitors":[]}`)
}

func TestSerpCompetitorsEmptyAndErrors(t *testing.T) {
	for _, tt := range []struct {
		name, result, message string
		status                int
	}{
		{"no result", `[]`, "Ok.", 20000}, {"null result", `null`, "Ok.", 20000},
		{"null first result", `[null]`, "Ok.", 20000}, {"missing items", `[{}]`, "Ok.", 20000}, {"null items", `[{"items":null}]`, "Ok.", 20000},
		{"no search results", `null`, "No Search Results.", 40501},
		{"invalid field", `null`, "Invalid Field: 'keywords'.", 40501}, {"provider error", `null`, "Internal SE Server Error.", 40101},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) { domainTestResponse(w, tt.status, tt.message, tt.result) })
			got, err := c.FindSerpCompetitors(context.Background(), FindSerpCompetitorsRequest{Keywords: []string{"a"}})
			if tt.status != 20000 {
				var apiErr *dataforseo.Error
				if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
					t.Fatalf("want task error %d, got %v", tt.status, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Competitors, []json.RawMessage{}) {
				t.Fatalf("competitors = %s", got.Competitors)
			}
		})
	}
}
