package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
)

func TestBusinessTaskToolSchemas(t *testing.T) {
	for _, tt := range []struct {
		name     string
		request  reflect.Type
		maxDepth float64
	}{
		{"get_business_reviews", reflect.TypeFor[seo.BusinessReviewsRequest](), 200},
		{"get_business_updates", reflect.TypeFor[seo.BusinessUpdatesRequest](), 100},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tool := keywordsToolByName(t, tt.name)
			var schema struct {
				Type       string                    `json:"type"`
				Properties map[string]map[string]any `json:"properties"`
				Required   []string                  `json:"required"`
				OneOf      []any                     `json:"oneOf"`
			}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Fatal(err)
			}
			// A call with only taskId is valid, so the schema cannot require an identifier.
			if schema.Type != "object" || schema.Required != nil || schema.OneOf != nil {
				t.Errorf("schema = %s", tool.InputSchema)
			}
			if got := kwJSONKeys(t, schema.Properties); !slices.Equal(got, kwJSONTags(tt.request)) {
				t.Errorf("schema fields %v, request fields %v", got, kwJSONTags(tt.request))
			}
			depth := schema.Properties["depth"]
			if depth["minimum"] != float64(10) || depth["maximum"] != tt.maxDepth {
				t.Errorf("depth = %v", depth)
			}
			taskID := schema.Properties["taskId"]
			if taskID["minLength"] != float64(1) || taskID["maxLength"] != float64(128) {
				t.Errorf("taskId = %v", taskID)
			}
			if tool.Title == "" || !strings.Contains(tool.Description, "billed by DataForSEO to the operator's account") || !strings.Contains(tool.Description, "no extra charge") {
				t.Errorf("description = %q", tool.Description)
			}
			for _, forbidden := range []string{"project", "credits", "cached", "openseo app"} {
				if strings.Contains(strings.ToLower(tool.Description+string(tool.InputSchema)), forbidden) {
					t.Errorf("tool contains %q", forbidden)
				}
			}
		})
	}
	sortBy := keywordsToolByName(t, "get_business_reviews").InputSchema
	var reviews struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(sortBy, &reviews); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reviews.Properties["sortBy"]["enum"], []any{"newest", "highest_rating", "lowest_rating", "relevant"}) {
		t.Error("sortBy enum mismatch")
	}
}

func TestBusinessTaskToolsCall(t *testing.T) {
	const id = "09271200-1535-0161-0000-772a4543ee8a"
	for _, tt := range []struct {
		name, args, postPath, getPath, task, result, want string
		resultType                                        reflect.Type
		posts                                             int32
	}{
		{
			"get_business_reviews", `{"businessName":"Example Cafe","depth":10,"sortBy":"highest_rating"}`,
			"/v3/business_data/google/reviews/task_post", "/v3/business_data/google/reviews/task_get/" + id,
			`[{"keyword":"Example Cafe","location_code":2840,"language_code":"en","depth":10,"sort_by":"highest_rating","priority":2}]`,
			`[{"title":"Example Cafe","reviews_count":1,"items":[{"review_text":"Good","xpath":"ignored"}]}]`,
			`{"status":"completed","taskId":"google:` + id + `","reviews":[{"review_text":"Good"}],"totals":{"title":"Example Cafe","reviews_count":1,"rating":null,"cid":null,"place_id":null}}`,
			reflect.TypeFor[*seo.BusinessReviewsResult](), 1,
		},
		{
			"get_business_updates", `{"taskId":"` + id + `"}`,
			"", "/v3/business_data/google/my_business_updates/task_get/" + id, "",
			`[{"items":[{"post_text":"Open late","images_url":"ignored"}]}]`,
			`{"status":"completed","taskId":"` + id + `","updates":[{"post_text":"Open late"}]}`,
			reflect.TypeFor[*seo.BusinessUpdatesResult](), 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodPost && r.URL.Path == tt.postPath:
					posts.Add(1)
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
					}
					businessToolJSON(t, json.RawMessage(raw), tt.task)
					_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"id":"` + id + `","status_code":20100,"result":null}]}`))
				case r.Method == http.MethodGet && r.URL.Path == tt.getPath:
					_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"id":"` + id + `","status_code":20000,"result":` + tt.result + `}]}`))
				default:
					t.Errorf("request = %s %s", r.Method, r.URL)
				}
			}))
			t.Cleanup(server.Close)
			api := dataforseo.New("synthetic-test-key")
			api.BaseURL = server.URL
			client := seo.New(api, seo.WithTaskPolling(time.Second, 10*time.Millisecond))
			got, err := New(client).Call(context.Background(), tt.name, json.RawMessage(tt.args))
			if err != nil {
				t.Fatal(err)
			}
			if reflect.TypeOf(got) != tt.resultType {
				t.Fatalf("result type = %T", got)
			}
			businessToolJSON(t, got, tt.want)
			if got := posts.Load(); got != tt.posts {
				t.Errorf("task_post requests = %d, want %d", got, tt.posts)
			}
		})
	}
}

func TestBusinessTaskToolArgumentErrors(t *testing.T) {
	for _, tt := range []struct{ name, args string }{
		{"get_business_reviews", `{}`},
		{"get_business_reviews", `{"cid":"1","depth":5}`},
		{"get_business_reviews", `{"cid":"1","sortBy":"oldest"}`},
		{"get_business_reviews", `{"taskId":"not-prefixed"}`},
		{"get_business_reviews", `{"cid":"1","includeOtherSources":"yes"}`},
		{"get_business_updates", `{"cid":"1","depth":101}`},
		{"get_business_updates", `{"taskId":"google:abc"}`},
		{"get_business_updates", `{"businessName":"Cafe","placeId":"p"}`},
	} {
		t.Run(tt.name+"/"+tt.args, func(t *testing.T) {
			_, err := New(seo.New(nil)).Call(context.Background(), tt.name, json.RawMessage(tt.args))
			var inputErr *seo.InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("want InputError, got %v", err)
			}
		})
	}
}
