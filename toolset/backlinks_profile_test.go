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

func TestBacklinksProfileToolSchema(t *testing.T) {
	props := domainToolSchema(t, backlinksProfileTool, "target")
	for field, want := range map[string][]any{
		"scope":     {"exact_url", "subfolder", "domain", "subdomains", "page"},
		"pageSize":  {float64(50), float64(100), float64(200)},
		"sortField": {"rank", "domainRank", "spamScore", "firstSeen"},
		"sortOrder": {"asc", "desc"}, "mode": {"one_per_domain", "as_is"},
	} {
		if !reflect.DeepEqual(props[field]["enum"], want) {
			t.Errorf("%s = %v", field, props[field])
		}
	}
	for field, want := range map[string]any{"page": float64(1), "pageSize": float64(100), "sortField": "firstSeen", "sortOrder": "desc", "mode": "one_per_domain", "hideSpam": true} {
		if props[field]["default"] != want {
			t.Errorf("%s default = %v", field, props[field]["default"])
		}
	}
	if props["target"]["maxLength"] != float64(2048) || props["page"]["minimum"] != float64(1) {
		t.Error("target or page bounds missing")
	}
	filters, ok := props["filters"]["properties"].(map[string]any)
	if !ok {
		t.Fatal("filters schema missing")
	}
	for _, field := range []string{"minDomainRank", "maxDomainRank", "minLinkAuthority", "maxLinkAuthority", "minSpamScore", "maxSpamScore"} {
		property, ok := filters[field].(map[string]any)
		if !ok || !reflect.DeepEqual(property["type"], []any{"number", "string"}) {
			t.Errorf("%s = %v", field, filters[field])
		}
		if _, ok := property["minimum"]; ok {
			t.Errorf("%s has an unsupported lower bound", field)
		}
		if _, ok := property["maximum"]; ok {
			t.Errorf("%s has an unsupported upper bound", field)
		}
	}
	var fields, filterFields []string
	for field := range props {
		fields = append(fields, field)
	}
	for field := range filters {
		filterFields = append(filterFields, field)
	}
	slices.Sort(fields)
	slices.Sort(filterFields)
	if want := kwJSONTags(reflect.TypeFor[seo.BacklinksProfileRequest]()); !reflect.DeepEqual(fields, want) {
		t.Errorf("schema fields=%v request fields=%v", fields, want)
	}
	if want := kwJSONTags(reflect.TypeFor[seo.BacklinksProfileFilters]()); !reflect.DeepEqual(filterFields, want) {
		t.Errorf("schema filters=%v request filters=%v", filterFields, want)
	}
}

func TestBacklinksProfileToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v3/backlinks/backlinks/live" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var got, want any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		if err := json.Unmarshal([]byte(`[{"target":"https://www.example.com/blog","include_subdomains":true,"include_indirect_links":true,"exclude_internal_backlinks":true,"backlinks_status_type":"live","rank_scale":"one_hundred","limit":200,"offset":200,"order_by":["domain_from_rank,asc"],"mode":"as_is","filters":[["rank",">=",12.5],"and",["dofollow","=",false]]}]`), &want); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("request body=%v want=%v", got, want)
		}
		_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"result":[{"total_count":201,"items":[{"url_from":"https://source.example/page","attributes":["nofollow"],"backlinks_spam_score":3,"lost_date":"lost"}]}]}]}`))
	}))
	defer server.Close()
	api := dataforseo.New("synthetic-test-key")
	api.BaseURL = server.URL
	result, err := New(seo.New(api)).Call(context.Background(), "get_backlinks_profile", json.RawMessage(`{"target":"https://www.example.com/blog","scope":"page","page":2,"pageSize":200,"sortField":"domainRank","sortOrder":"asc","mode":"as_is","hideSpam":false,"filters":{"minLinkAuthority":"12.5","linkType":"nofollow"}}`))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := result.(*seo.BacklinksProfileResult)
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if got.Target != "https://www.example.com/blog" || got.Scope != "exact_url" || got.Backlinks.Page != 2 || got.Backlinks.PageSize != 200 || got.Backlinks.HasMore || len(got.Backlinks.Rows) != 1 {
		t.Fatalf("result = %+v", got)
	}
	row := got.Backlinks.Rows[0]
	if row.URLFrom == nil || *row.URLFrom != "https://source.example/page" || !row.IsLost || row.SpamScore == nil || *row.SpamScore != 3 || !reflect.DeepEqual(row.RelAttributes, []string{"nofollow"}) {
		t.Fatalf("row = %+v", row)
	}
	if want := []string{"backlinks", "scope", "target"}; !reflect.DeepEqual(kwJSONKeys(t, got), want) {
		t.Errorf("result keys = %v", kwJSONKeys(t, got))
	}
}

func TestBacklinksProfileToolInputErrors(t *testing.T) {
	for _, args := range []string{
		`{}`, `{"target":42}`, `{"target":"example.com","page":0}`, `{"target":"example.com","page":1.5}`,
		`{"target":"example.com","pageSize":20}`, `{"target":"example.com","hideSpam":"false"}`, `{"target":"example.com","filters":{"minDomainRank":"bad"}}`,
		`{"target":"example.com","filters":{"maxSpamScore":null}}`, `{"target":"example.com","filters":{"hideLost":"true"}}`, `{"target":"example.com","mode":"bad"}`,
	} {
		_, err := New(seo.New(nil)).Call(context.Background(), "get_backlinks_profile", json.RawMessage(args))
		var inputErr *seo.InputError
		if !errors.As(err, &inputErr) {
			t.Errorf("%s: %v", args, err)
		}
	}
}
