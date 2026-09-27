package seo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

func TestBacklinksOverviewRequestsAndShape(t *testing.T) {
	summary := domainTestFixture(t, "testdata/backlinks/summary.json")
	history := domainTestFixture(t, "testdata/backlinks/history.json")
	refs := domainTestFixture(t, "testdata/backlinks/referring_domains.json")
	for _, tt := range []struct {
		name                   string
		req                    BacklinksOverviewRequest
		api, scope             string
		include, spam, history bool
	}{
		{"default", BacklinksOverviewRequest{Target: "Example.com"}, "example.com", "subdomains", true, true, true},
		{"domain no spam", BacklinksOverviewRequest{Target: "www.example.com", Scope: "domain", HideSpam: domainTestPtr(false)}, "example.com", "domain", false, false, true},
		{"page", BacklinksOverviewRequest{Target: "http://www.example.com/page/", Scope: "exact_url", HideSpam: domainTestPtr(true)}, "http://www.example.com/page", "exact_url", true, true, false},
		{"legacy page", BacklinksOverviewRequest{Target: "example.com", Scope: "page"}, "https://example.com/", "exact_url", true, true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			calls := map[string]int{}
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calls[r.URL.Path]++
				mu.Unlock()
				common := map[string]any{"target": tt.api, "include_subdomains": tt.include, "include_indirect_links": true, "exclude_internal_backlinks": true, "backlinks_status_type": "live", "rank_scale": "one_hundred"}
				response := "null"
				switch r.URL.Path {
				case "/v3/backlinks/summary/live":
					response = summary
				case "/v3/backlinks/referring_domains/live":
					common["limit"], common["offset"], common["order_by"] = 100, 0, []string{"backlinks,desc"}
					if tt.spam {
						common["filters"] = []any{[]any{"backlinks_spam_score", "<=", 40}}
					}
					response = refs
				case "/v3/backlinks/history/live":
					if !tt.history {
						t.Error("page requested history")
					}
					now := time.Now().UTC()
					to := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)
					from := time.Date(to.Year()-1, to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
					common = map[string]any{"target": tt.api, "date_from": from.Format("2006-01-02"), "date_to": to.Format("2006-01-02"), "rank_scale": "one_hundred"}
					response = history
				default:
					t.Errorf("unexpected path %s", r.URL.Path)
				}
				payload, _ := json.Marshal(common)
				domainTestBody(t, r, string(payload))
				domainTestResponse(w, 20000, "Ok.", response)
			})
			got, err := c.BacklinksOverview(context.Background(), tt.req)
			if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			wantCalls := 2
			if tt.history {
				wantCalls = 3
			}
			if len(calls) != wantCalls {
				t.Errorf("calls = %v", calls)
			}
			for path, count := range calls {
				if count != 1 {
					t.Errorf("%s called %d times", path, count)
				}
			}
			mu.Unlock()
			profile := &got.Overview.Overview
			if got.Target != tt.api || got.Scope != tt.scope || profile.Target != tt.api || profile.DisplayTarget != tt.api || profile.Scope != tt.scope {
				t.Fatalf("target mismatch: %+v", got)
			}
			domainTestJSON(t, profile.Summary, `{"rank":33.5,"backlinks":12,"referringPages":9,"referringDomains":4,"brokenBacklinks":2,"brokenPages":1,"backlinksSpamScore":7,"targetSpamScore":8,"newBacklinks":null,"lostBacklinks":null,"newReferringDomains":0,"lostReferringDomains":3}`)
			wantTrends := `[]`
			wantNewLost := `[]`
			if tt.history {
				wantTrends = `[{"date":"2025-04-02","backlinks":10,"referringDomains":3,"rank":31},{"date":"short","backlinks":null,"referringDomains":null,"rank":null}]`
				wantNewLost = `[{"date":"2025-04-02","newBacklinks":2,"lostBacklinks":1,"newReferringDomains":4,"lostReferringDomains":0},{"date":"short","newBacklinks":null,"lostBacklinks":null,"newReferringDomains":null,"lostReferringDomains":null}]`
			}
			domainTestJSON(t, profile.Trends, wantTrends)
			domainTestJSON(t, profile.NewLostTrends, wantNewLost)
			for _, timestamp := range []string{profile.FetchedAt, got.ReferringDomains.FetchedAt} {
				if _, err := time.Parse("2006-01-02T15:04:05.000Z", timestamp); err != nil {
					t.Fatal(err)
				}
			}
			got.ReferringDomains.FetchedAt = "timestamp"
			domainTestJSON(t, got.ReferringDomains, `{"rows":[{"domain":"source.example","backlinks":8,"referringPages":6,"rank":22,"spamScore":0,"firstSeen":"2025-01-02 03:04:05 +00:00","brokenBacklinks":0,"brokenPages":1},{"domain":null,"backlinks":null,"referringPages":null,"rank":null,"spamScore":null,"firstSeen":null,"brokenBacklinks":null,"brokenPages":null}],"totalCount":105,"hasMore":true,"page":1,"pageSize":100,"fetchedAt":"timestamp"}`)
			raw, _ := json.Marshal(got)
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(raw, &fields)
			wantFields := 4
			if tt.scope == "domain" {
				wantFields = 5
				if got.ScopeNote != "Summary excludes subdomains; trend data includes subdomains (provider limitation)." {
					t.Errorf("scope note = %s", got.ScopeNote)
				}
			}
			if len(fields) != wantFields {
				t.Errorf("outer shape = %s", raw)
			}
			var wrapper map[string]json.RawMessage
			_ = json.Unmarshal(fields["overview"], &wrapper)
			if len(wrapper) != 1 || wrapper["overview"] == nil {
				t.Errorf("overview wrapper = %s", fields["overview"])
			}
		})
	}
}

func TestBacklinksSubfolder(t *testing.T) {
	for _, hideSpam := range []*bool{nil, domainTestPtr(false), domainTestPtr(true)} {
		var calls atomic.Int32
		c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			n := calls.Add(1)
			mode := "as_is"
			total := 12
			if n == 2 {
				mode = "one_per_domain"
				total = 4
			}
			if r.URL.Path != "/v3/backlinks/backlinks/live" {
				t.Errorf("path = %s", r.URL.Path)
			}
			want := `{"target":"example.com","include_subdomains":false,"include_indirect_links":true,"exclude_internal_backlinks":true,"backlinks_status_type":"live","rank_scale":"one_hundred","limit":1,"mode":"` + mode + `","order_by":["rank,desc"],"filters":[[["url_to","like","%://example.com/a\\_b\\%20c"],"or",["url_to","like","%://example.com/a\\_b\\%20c/%"],"or",["url_to","like","%://www.example.com/a\\_b\\%20c"],"or",["url_to","like","%://www.example.com/a\\_b\\%20c/%"]],"and",["backlink_spam_score","<=",40]]}`
			domainTestBody(t, r, want)
			response, _ := json.Marshal([]any{map[string]any{"total_count": total, "items": []any{}}})
			domainTestResponse(w, 20000, "Ok.", string(response))
		})
		got, err := c.BacklinksOverview(context.Background(), BacklinksOverviewRequest{Target: "https://www.example.com/a_b%20c/?q=1#top", HideSpam: hideSpam})
		if err != nil {
			t.Fatal(err)
		}
		if calls.Load() != 2 {
			t.Errorf("calls = %d", calls.Load())
		}
		got.Overview.Overview.FetchedAt = "timestamp"
		domainTestJSON(t, got, `{"target":"example.com/a_b%20c","scope":"subfolder","scopeNote":"Counts are computed from filtered backlink totals; rank, trends, and the referring-domains breakdown aren't available for subfolders.","overview":{"overview":{"target":"example.com","displayTarget":"example.com/a_b%20c","scope":"subfolder","summary":{"rank":null,"backlinks":12,"referringPages":null,"referringDomains":4,"brokenBacklinks":null,"brokenPages":null,"backlinksSpamScore":null,"targetSpamScore":null,"newBacklinks":null,"lostBacklinks":null,"newReferringDomains":null,"lostReferringDomains":null},"trends":[],"newLostTrends":[],"fetchedAt":"timestamp"}}}`)
	}
}

func TestBacklinksEmptyAndPaginationFallback(t *testing.T) {
	for _, tt := range []struct {
		name, result string
		count        int
		total        *float64
		more         bool
	}{
		{"null result", `null`, 0, nil, false}, {"empty result", `[]`, 0, nil, false}, {"null items", `[{"items":null}]`, 0, nil, false},
		{"null count", `[{"items":[{}],"total_count":null}]`, 1, nil, false}, {"invalid count", `[{"items":[],"total_count":"bad"}]`, 0, nil, false},
		{"full page unknown count", `[{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 100), ",") + `]}]`, 100, nil, true},
		{"full page known count", `[{"items":[` + strings.TrimSuffix(strings.Repeat(`{},`, 100), ",") + `],"total_count":100}]`, 100, domainTestPtr(100.0), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				result := "null"
				if strings.Contains(r.URL.Path, "referring_domains") {
					result = tt.result
				}
				domainTestResponse(w, 20000, "Ok.", result)
			})
			got, err := c.BacklinksOverview(context.Background(), BacklinksOverviewRequest{Target: "example.com"})
			if err != nil {
				t.Fatal(err)
			}
			page := got.ReferringDomains
			if len(page.Rows) != tt.count || page.HasMore != tt.more || (page.TotalCount == nil) != (tt.total == nil) {
				t.Fatalf("page = %+v", page)
			}
			if tt.total != nil && *page.TotalCount != *tt.total {
				t.Errorf("total = %v", page.TotalCount)
			}
			domainTestJSON(t, got.Overview.Overview.Summary, `{"rank":null,"backlinks":null,"referringPages":null,"referringDomains":null,"brokenBacklinks":null,"brokenPages":null,"backlinksSpamScore":null,"targetSpamScore":null,"newBacklinks":null,"lostBacklinks":null,"newReferringDomains":null,"lostReferringDomains":null}`)
			domainTestJSON(t, got.Overview.Overview.Trends, `[]`)
			domainTestJSON(t, got.Overview.Overview.NewLostTrends, `[]`)
		})
	}
	for _, result := range []string{`null`, `[]`, `[{"total_count":null}]`, `[{"total_count":"bad"}]`} {
		c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) { domainTestResponse(w, 20000, "Ok.", result) })
		got, err := c.BacklinksOverview(context.Background(), BacklinksOverviewRequest{Target: "example.com/blog"})
		if err != nil {
			t.Fatal(err)
		}
		if got.Overview.Overview.Summary.Backlinks != nil || got.Overview.Overview.Summary.ReferringDomains != nil {
			t.Fatalf("counts = %+v", got.Overview.Overview.Summary)
		}
	}
}

func TestBacklinksTaskErrors(t *testing.T) {
	for _, endpoint := range []string{"summary", "history", "referring_domains", "backlinks"} {
		for _, tt := range []struct {
			status  int
			message string
		}{{40501, "No Search Results."}, {40501, "Invalid Field: 'target'."}, {40101, "Internal SE Server Error."}} {
			t.Run(endpoint+tt.message, func(t *testing.T) {
				c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/v3/backlinks/"+endpoint+"/live" {
						domainTestResponse(w, tt.status, tt.message, "null")
					} else {
						domainTestResponse(w, 20000, "Ok.", "null")
					}
				})
				target := "example.com"
				if endpoint == "backlinks" {
					target += "/blog"
				}
				_, err := c.BacklinksOverview(context.Background(), BacklinksOverviewRequest{Target: target})
				var apiErr *dataforseo.Error
				if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
					t.Fatalf("error = %v", err)
				}
			})
		}
	}
}

func TestBacklinksValidationAndDateRange(t *testing.T) {
	for _, req := range []BacklinksOverviewRequest{{}, {Target: "example.por"}, {Target: "example.com", Scope: "bad"}, {Target: "example.com", Scope: "subfolder"}, {Target: "example.com/page?x=1", Scope: "exact_url"}} {
		_, err := New(nil).BacklinksOverview(context.Background(), req)
		var inputErr *InputError
		if !errors.As(err, &inputErr) {
			t.Errorf("request %+v: error = %v", req, err)
		}
	}
	for _, tt := range []struct{ now, from, to string }{{"2024-03-01", "2023-03-01", "2024-02-29"}, {"2025-01-01", "2023-12-31", "2024-12-31"}, {"2025-06-12", "2024-06-11", "2025-06-11"}} {
		now, _ := time.Parse("2006-01-02", tt.now)
		from, to := backlinksDateRange(now)
		if from != tt.from || to != tt.to {
			t.Errorf("%s: got %s to %s", tt.now, from, to)
		}
	}
}
