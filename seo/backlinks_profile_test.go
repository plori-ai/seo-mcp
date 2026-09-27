package seo

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

// Expected common fields are independent of the production payload helper.
func backlinksProfileTestTask(t *testing.T, overrides string) string {
	t.Helper()
	var task, changes map[string]json.RawMessage
	if err := json.Unmarshal([]byte(`{"target":"example.com","include_subdomains":true,"include_indirect_links":true,"exclude_internal_backlinks":true,"backlinks_status_type":"live","rank_scale":"one_hundred","limit":100,"offset":0,"order_by":["first_seen,desc"],"mode":"one_per_domain","filters":[["backlink_spam_score","<=",40]]}`), &task); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(overrides), &changes); err != nil {
		t.Fatal(err)
	}
	for key, value := range changes {
		if string(value) == "null" {
			delete(task, key)
		} else {
			task[key] = value
		}
	}
	raw, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestBacklinksProfileRequestsAndShape(t *testing.T) {
	fixture := domainTestFixture(t, "testdata/backlinks_profile/items.json")
	wantRows := domainTestFixture(t, "testdata/backlinks_profile/rows_want.json")
	for _, tt := range []struct {
		name, args, overrides, target, scope string
		page, pageSize                       int
		hasMore                              bool
	}{
		{"defaults", `{"target":"https://www.Example.com/"}`, `{}`, "example.com", "subdomains", 1, 100, true},
		{"domain and disabled spam", `{"target":"example.com","scope":"domain","page":2,"pageSize":50,"sortField":"rank","sortOrder":"asc","mode":"as_is","hideSpam":false}`, `{"include_subdomains":false,"limit":50,"offset":50,"order_by":["rank,asc"],"mode":"as_is","filters":null}`, "example.com", "domain", 2, 50, false},
		{"exact URL", `{"target":"http://www.Example.com/Blog/","scope":"exact_url","pageSize":200,"sortField":"domainRank"}`, `{"target":"http://www.example.com/Blog","limit":200,"order_by":["domain_from_rank,desc"]}`, "http://www.example.com/Blog", "exact_url", 1, 200, true},
		{"legacy page", `{"target":"example.com","scope":"page","sortField":"spamScore","sortOrder":"asc"}`, `{"target":"https://example.com/","order_by":["backlink_spam_score,asc"]}`, "https://example.com/", "exact_url", 1, 100, true},
		{"subfolder and scope escaping", `{"target":"https://www.Example.com/a_b%20c/?q=1#top","filters":{"include":"Blog"}}`, `{"include_subdomains":false,"filters":[[["url_to","like","%://example.com/a\\_b\\%20c"],"or",["url_to","like","%://example.com/a\\_b\\%20c/%"],"or",["url_to","like","%://www.example.com/a\\_b\\%20c"],"or",["url_to","like","%://www.example.com/a\\_b\\%20c/%"]],"and",["url_from","ilike","%blog%"],"and",["backlink_spam_score","<=",40]]}`, "example.com/a_b%20c", "subfolder", 1, 100, true},
		{"root overrides path", `{"target":"example.com/blog","scope":"subdomains","page":3,"pageSize":200}`, `{"offset":400,"limit":200}`, "example.com", "subdomains", 3, 200, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantTask := backlinksProfileTestTask(t, tt.overrides)
			var calls atomic.Int32
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v3/backlinks/backlinks/live" {
					t.Errorf("path = %s", r.URL.Path)
				}
				domainTestBody(t, r, wantTask)
				domainTestResponse(w, 20000, "Ok.", fixture)
			})
			var req BacklinksProfileRequest
			if err := json.Unmarshal([]byte(tt.args), &req); err != nil {
				t.Fatal(err)
			}
			got, err := c.BacklinksProfile(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if calls.Load() != 1 {
				t.Errorf("calls = %d", calls.Load())
			}
			if _, err := time.Parse("2006-01-02T15:04:05.000Z", got.Backlinks.FetchedAt); err != nil {
				t.Fatal(err)
			}
			want, err := json.Marshal(map[string]any{
				"target": tt.target, "scope": tt.scope, "backlinks": map[string]any{
					"rows": json.RawMessage(wantRows), "totalCount": 12, "hasMore": tt.hasMore,
					"page": tt.page, "pageSize": tt.pageSize, "fetchedAt": got.Backlinks.FetchedAt,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			domainTestJSON(t, got, string(want))
		})
	}
}

func TestBacklinksProfileFilters(t *testing.T) {
	for _, tt := range []struct{ name, args, overrides string }{
		{"include OR and exclusions", `{"target":"example.com","filters":{"include":" Blog, News+ ,","exclude":"SPAM, Bad","minDomainRank":30,"linkType":"dofollow","hideLost":true}}`, `{"filters":[[["url_from","ilike","%blog%"],"or",["url_from","ilike","%news%"]],"and",["url_from","not_ilike","%spam%"],"and",["url_from","not_ilike","%bad%"],"and",["domain_from_rank",">=",30],"and",["dofollow","=",true],"and",["is_lost","=",false],"and",["backlink_spam_score","<=",40]]}`},
		{"term escaping", `{"target":"example.com","hideSpam":false,"filters":{"include":" WP_Content\\100% ","exclude":"A_B%"}}`, `{"filters":[["url_from","ilike","%wp\\_content\\\\100\\%%"],"and",["url_from","not_ilike","%a\\_b\\%%"]]}`},
		{"all numeric bounds", `{"target":"example.com","filters":{"minDomainRank":" -1.5 ","maxDomainRank":"1000","minLinkAuthority":0,"maxLinkAuthority":"1e2","minSpamScore":".5","maxSpamScore":"+99."}}`, `{"filters":[["domain_from_rank",">=",-1.5],"and",["domain_from_rank","<=",1000],"and",["rank",">=",0],"and",["rank","<=",100],"and",["backlink_spam_score",">=",0.5],"and",["backlink_spam_score","<=",99],"and",["backlink_spam_score","<=",40]]}`},
		{"number radix strings and blanks", `{"target":"example.com","hideSpam":false,"filters":{"minDomainRank":"0x10","maxDomainRank":"0b100000","minLinkAuthority":"0o10","maxLinkAuthority":" ","minSpamScore":"","maxSpamScore":"0X2A"}}`, `{"filters":[["domain_from_rank",">=",16],"and",["domain_from_rank","<=",32],"and",["rank",">=",8],"and",["backlink_spam_score","<=",42]]}`},
		{"status and exact domain", `{"target":"example.com","hideSpam":false,"filters":{"linkType":"nofollow","hideLost":false,"hideBroken":true,"domainFrom":"Source.Example"}}`, `{"filters":[["dofollow","=",false],"and",["is_broken","=",false],"and",["domain_from","=","Source.Example"]]}`},
		{"empty filters omitted", `{"target":"example.com","hideSpam":false,"filters":{"include":" , + ","exclude":"","minDomainRank":" ","hideLost":false,"hideBroken":false,"domainFrom":""}}`, `{"filters":null}`},
		{"eight user conditions without spam", `{"target":"example.com","hideSpam":false,"filters":{"minDomainRank":1,"maxDomainRank":2,"minLinkAuthority":3,"maxLinkAuthority":4,"minSpamScore":5,"maxSpamScore":6,"hideLost":true,"hideBroken":true}}`, `{"filters":[["domain_from_rank",">=",1],"and",["domain_from_rank","<=",2],"and",["rank",">=",3],"and",["rank","<=",4],"and",["backlink_spam_score",">=",5],"and",["backlink_spam_score","<=",6],"and",["is_lost","=",false],"and",["is_broken","=",false]]}`},
		{"subfolder combined budget", `{"target":"example.com/blog","filters":{"minDomainRank":1,"hideLost":true,"hideBroken":true}}`, `{"include_subdomains":false,"filters":[[["url_to","like","%://example.com/blog"],"or",["url_to","like","%://example.com/blog/%"],"or",["url_to","like","%://www.example.com/blog"],"or",["url_to","like","%://www.example.com/blog/%"]],"and",["domain_from_rank",">=",1],"and",["is_lost","=",false],"and",["is_broken","=",false],"and",["backlink_spam_score","<=",40]]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wantTask := backlinksProfileTestTask(t, tt.overrides)
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				domainTestBody(t, r, wantTask)
				domainTestResponse(w, 20000, "Ok.", `[]`)
			})
			var req BacklinksProfileRequest
			if err := json.Unmarshal([]byte(tt.args), &req); err != nil {
				t.Fatal(err)
			}
			if _, err := c.BacklinksProfile(context.Background(), req); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBacklinksProfilePagination(t *testing.T) {
	for _, tt := range []struct {
		name, total string
		page, rows  int
		hasMore     bool
	}{
		{"known more", `101`, 2, 50, true}, {"known last page", `100`, 2, 50, false},
		{"known partial last page", `55`, 2, 5, false}, {"known zero", `0`, 1, 0, false},
		{"known total with empty page", `101`, 2, 0, true},
		{"null total full", `null`, 2, 50, true}, {"null total partial", `null`, 2, 49, false},
		{"missing total full", ``, 1, 50, true}, {"string total ignored", `"100"`, 2, 50, true},
		{"object total ignored", `{}`, 1, 0, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			items := make([]map[string]any, tt.rows)
			for i := range items {
				items[i] = map[string]any{}
			}
			response := map[string]any{"items": items}
			if tt.total != "" {
				response["total_count"] = json.RawMessage(tt.total)
			}
			raw, err := json.Marshal([]any{response})
			if err != nil {
				t.Fatal(err)
			}
			overrides, err := json.Marshal(map[string]any{"limit": 50, "offset": (tt.page - 1) * 50})
			if err != nil {
				t.Fatal(err)
			}
			wantTask := backlinksProfileTestTask(t, string(overrides))
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				domainTestBody(t, r, wantTask)
				domainTestResponse(w, 20000, "Ok.", string(raw))
			})
			got, err := c.BacklinksProfile(context.Background(), BacklinksProfileRequest{Target: "example.com", Page: domainTestPtr(tt.page), PageSize: domainTestPtr(50)})
			if err != nil {
				t.Fatal(err)
			}
			if got.Backlinks.HasMore != tt.hasMore || got.Backlinks.Page != tt.page || got.Backlinks.PageSize != 50 || len(got.Backlinks.Rows) != tt.rows {
				t.Fatalf("page = %+v", got.Backlinks)
			}
			if tt.total == "" || tt.total == "null" || tt.total == `"100"` || tt.total == `{}` {
				if got.Backlinks.TotalCount != nil {
					t.Fatalf("totalCount = %v", got.Backlinks.TotalCount)
				}
			} else {
				domainTestJSON(t, got.Backlinks.TotalCount, tt.total)
			}
		})
	}
}

func TestBacklinksProfileInputValidation(t *testing.T) {
	for _, args := range []string{
		`{}`, `{"target":"example.por"}`, `{"target":"example.com","scope":"bad"}`, `{"target":"example.com","scope":"subfolder"}`,
		`{"target":"https://example.com/?q=1","scope":"page"}`, `{"target":"https://example.com/blog#top","scope":"exact_url"}`,
		`{"target":"example.com","page":0}`, `{"target":"example.com","page":-1}`,
		`{"target":"example.com","pageSize":0}`, `{"target":"example.com","pageSize":51}`, `{"target":"example.com","pageSize":201}`,
		`{"target":"example.com","sortField":"spam_score"}`, `{"target":"example.com","sortOrder":"bad"}`, `{"target":"example.com","mode":"one_per_anchor"}`,
		`{"target":"example.com","filters":{"linkType":"all"}}`,
		`{"target":"example.com","filters":{"include":"a,b,c,d,e,f,g,h"}}`,
		`{"target":"example.com","hideSpam":false,"filters":{"include":"a,b,c,d,e,f,g,h,i"}}`,
		`{"target":"example.com/blog","filters":{"include":"a,b,c,d"}}`,
		`{"target":"example.com/blog","hideSpam":false,"filters":{"include":"a,b,c,d,e"}}`,
	} {
		t.Run(args, func(t *testing.T) {
			var req BacklinksProfileRequest
			if err := json.Unmarshal([]byte(args), &req); err != nil {
				t.Fatal(err)
			}
			_, err := New(nil).BacklinksProfile(context.Background(), req)
			var inputErr *InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("want InputError, got %v", err)
			}
		})
	}
	for _, req := range []BacklinksProfileRequest{
		{Target: strings.Repeat("x", 2049)},
		{Target: "example.com", Filters: BacklinksProfileFilters{DomainFrom: strings.Repeat("x", 256)}},
		{Target: "example.com", Filters: BacklinksProfileFilters{MinDomainRank: domainTestPtr(math.NaN())}},
		{Target: "example.com", Filters: BacklinksProfileFilters{MaxSpamScore: domainTestPtr(math.Inf(1))}},
		{Target: "example.com", Page: domainTestPtr(int(^uint(0) >> 1))},
	} {
		_, err := New(nil).BacklinksProfile(context.Background(), req)
		var inputErr *InputError
		if !errors.As(err, &inputErr) {
			t.Errorf("request %+v: want InputError, got %v", req, err)
		}
	}
}

func TestBacklinksProfileInvalidNumericStrings(t *testing.T) {
	for _, raw := range []string{`"bad"`, `"NaN"`, `"Infinity"`, `"1e999"`, `"1_000"`, `"0x"`, `"0x+1"`, `"0x1p2"`, `"-0x1"`, `"0b2"`, `true`, `null`, `{}`, `[]`} {
		t.Run(raw, func(t *testing.T) {
			var req BacklinksProfileRequest
			err := json.Unmarshal([]byte(`{"target":"example.com","filters":{"minDomainRank":`+raw+`}}`), &req)
			var inputErr *InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("want InputError, got %v", err)
			}
		})
	}
}

func TestBacklinksProfileEmptyAndErrors(t *testing.T) {
	for _, tt := range []struct {
		name, result, message string
		status                int
	}{
		{"no result", `[]`, "Ok.", 20000}, {"null result", `null`, "Ok.", 20000}, {"null first result", `[null]`, "Ok.", 20000},
		{"missing items", `[{}]`, "Ok.", 20000}, {"null items", `[{"items":null}]`, "Ok.", 20000}, {"empty items", `[{"items":[]}]`, "Ok.", 20000},
		{"no search results", `null`, "No Search Results.", 40501},
		{"invalid target", `null`, "Invalid Field: 'target'.", 40501}, {"provider error", `null`, "Internal SE Server Error.", 40101},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) { domainTestResponse(w, tt.status, tt.message, tt.result) })
			got, err := c.BacklinksProfile(context.Background(), BacklinksProfileRequest{Target: "example.com"})
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
			domainTestJSON(t, got, `{"target":"example.com","scope":"subdomains","backlinks":{"rows":[],"totalCount":null,"hasMore":false,"page":1,"pageSize":100,"fetchedAt":"`+got.Backlinks.FetchedAt+`"}}`)
		})
	}
}

func TestBacklinksProfileInvalidProviderRows(t *testing.T) {
	for _, result := range []string{`[{"items":[null]}]`, `[{"items":[{"rank":"bad"}]}]`, `[{"items":{}}]`, `[{"items":[{"rel_attributes":"nofollow"}]}]`} {
		t.Run(result, func(t *testing.T) {
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) { domainTestResponse(w, 20000, "Ok.", result) })
			_, err := c.BacklinksProfile(context.Background(), BacklinksProfileRequest{Target: "example.com"})
			if err == nil {
				t.Fatal("want invalid provider response error")
			}
		})
	}
}
