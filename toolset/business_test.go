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
	"testing"

	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
)

func businessToolJSON(t *testing.T, got any, want string) {
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

func TestBusinessToolSchemas(t *testing.T) {
	for _, tt := range []struct {
		name             string
		request          reflect.Type
		required         []string
		identifier, paid bool
	}{
		{"search_local_businesses", reflect.TypeFor[seo.SearchLocalBusinessesRequest](), []string{"near"}, false, true},
		{"list_business_categories", reflect.TypeFor[seo.ListBusinessCategoriesRequest](), nil, false, false},
		{"get_business_profile", reflect.TypeFor[seo.BusinessProfileRequest](), nil, true, true},
		{"get_google_business_questions", reflect.TypeFor[seo.BusinessQuestionsRequest](), []string{"near"}, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tool := keywordsToolByName(t, tt.name)
			var schema struct {
				Type       string                    `json:"type"`
				Properties map[string]map[string]any `json:"properties"`
				Required   []string                  `json:"required"`
				OneOf      []map[string][]string     `json:"oneOf"`
			}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Fatal(err)
			}
			if schema.Type != "object" || !slices.Equal(schema.Required, tt.required) {
				t.Errorf("schema = %s", tool.InputSchema)
			}
			if got := kwJSONKeys(t, schema.Properties); !slices.Equal(got, kwJSONTags(tt.request)) {
				t.Errorf("schema fields %v, request fields %v", got, kwJSONTags(tt.request))
			}
			if tt.identifier {
				want := []map[string][]string{{"required": {"businessName"}}, {"required": {"cid"}}, {"required": {"placeId"}}}
				if !reflect.DeepEqual(schema.OneOf, want) {
					t.Errorf("oneOf = %v", schema.OneOf)
				}
				for field, maximum := range map[string]float64{"businessName": 200, "cid": 64, "placeId": 256} {
					if schema.Properties[field]["minLength"] != float64(1) || schema.Properties[field]["maxLength"] != maximum {
						t.Errorf("%s constraints = %v", field, schema.Properties[field])
					}
				}
			}
			if tool.Title == "" {
				t.Error("missing title")
			}
			if tt.paid {
				if !strings.Contains(tool.Description, "billed by DataForSEO to the operator's account") {
					t.Error("missing operator billing description")
				}
			} else if !strings.Contains(tool.Description, "DataForSEO does not charge") {
				t.Error("missing free endpoint description")
			}
			for _, forbidden := range []string{"project", "credits", "cached", "openseo app"} {
				if strings.Contains(strings.ToLower(tool.Description+string(tool.InputSchema)), forbidden) {
					t.Errorf("tool contains %q", forbidden)
				}
			}
			if near, ok := schema.Properties["near"]; ok {
				nearProps := near["properties"].(map[string]any)
				latitude := nearProps["latitude"].(map[string]any)
				longitude := nearProps["longitude"].(map[string]any)
				if latitude["minimum"] != float64(-90) || latitude["maximum"] != float64(90) || longitude["minimum"] != float64(-180) || longitude["maximum"] != float64(180) {
					t.Errorf("coordinate bounds = %v", nearProps)
				}
				radius := nearProps["radiusKm"].(map[string]any)
				wantMin, wantMax := 1.0, 100000.0
				wantRequired := []any{"latitude", "longitude", "radiusKm"}
				if tt.name == "get_business_profile" {
					wantMin, wantMax = 0.2, 199
					wantRequired = []any{"latitude", "longitude"}
				}
				if radius["minimum"] != wantMin || radius["maximum"] != wantMax || !reflect.DeepEqual(near["required"], wantRequired) {
					t.Errorf("near schema = %v", near)
				}
			}
			for _, bound := range []struct {
				field            string
				minimum, maximum float64
			}{
				{"limit", 1, map[string]float64{"search_local_businesses": 50, "list_business_categories": 200}[tt.name]},
				{"offset", 0, 1000}, {"depth", 1, 100}, {"minRating", 1, 5},
			} {
				if property, ok := schema.Properties[bound.field]; ok {
					if property["minimum"] != bound.minimum || property["maximum"] != bound.maximum {
						t.Errorf("%s constraints = %v", bound.field, property)
					}
				}
			}
			if tt.name == "search_local_businesses" {
				categories := schema.Properties["categories"]
				item := categories["items"].(map[string]any)
				if categories["minItems"] != float64(1) || categories["maxItems"] != float64(10) || item["minLength"] != float64(1) || item["maxLength"] != float64(120) {
					t.Errorf("category constraints = %v", categories)
				}
				if !reflect.DeepEqual(schema.Properties["sortBy"]["enum"], []any{"relevance", "rating", "reviews"}) {
					t.Error("sortBy enum mismatch")
				}
			}
		})
	}
}

func TestBusinessToolsCall(t *testing.T) {
	for _, tt := range []struct {
		name, args, method, path, task, response, want string
		resultType                                     reflect.Type
	}{
		{
			"search_local_businesses",
			`{"query":"Cafe","near":{"latitude":33,"longitude":-84,"radiusKm":1.5},"categories":["cafe"],"minRating":4,"minReviews":0,"isClaimed":false,"sortBy":"reviews","limit":50,"offset":0}`,
			http.MethodPost, "/v3/business_data/business_listings/search/live",
			`[{"title":"Cafe","location_coordinate":"33,-84,2","categories":["cafe"],"filters":[["rating.value",">=",4],"and",["rating.votes_count",">=",0]],"is_claimed":false,"order_by":["rating.votes_count,desc"],"limit":50,"offset":0}]`,
			`[{"items":[{"title":"Cafe","phone":null,"rating":{"value":4.8,"votes_count":100},"is_claimed":false,"popular_times":{"ignored":true}}]}]`,
			`{"businesses":[{"title":"Cafe","phone":null,"rating":{"value":4.8,"votes_count":100},"is_claimed":false}]}`,
			reflect.TypeFor[*seo.SearchLocalBusinessesResult](),
		},
		{
			"list_business_categories", `{"query":"plUm","limit":2}`,
			http.MethodGet, "/v3/business_data/business_listings/categories", "",
			`[{"category_name":"plumbing_supply","business_count":20},{"category_name":"cafe","business_count":500},{"category_name":"Plumber","business_count":100}]`,
			`{"categories":[{"category":"Plumber","businessCount":100},{"category":"plumbing_supply","businessCount":20}]}`,
			reflect.TypeFor[*seo.ListBusinessCategoriesResult](),
		},
		{
			"get_business_profile", `{"cid":"123","locationCode":2276,"languageCode":"de"}`,
			http.MethodPost, "/v3/business_data/google/my_business_info/live",
			`[{"keyword":"cid:123","location_code":2276,"language_code":"de"}]`,
			`[{"check_url":"https://maps.example/cafe","items":[{"title":"Cafe","work_time":{"kept":true}}]}]`,
			`{"profile":{"title":"Cafe","check_url":"https://maps.example/cafe","work_time":{"kept":true}}}`,
			reflect.TypeFor[*seo.BusinessProfileResult](),
		},
		{
			"get_google_business_questions", `{"placeId":"place","near":{"latitude":33,"longitude":-84,"radiusKm":1000},"languageCode":"es","depth":100}`,
			http.MethodPost, "/v3/business_data/google/questions_and_answers/live",
			`[{"keyword":"place_id:place","location_coordinate":"33,-84,199999","language_code":"es","depth":100}]`,
			`[{"items":[{"question_text":"Open Sunday?","url":"ignored","items":[{"answer_text":"Yes","profile_url":"ignored"}]}]},{"items_without_answers":[{"question_text":"Parking?"}]}]`,
			`{"questions":[{"question_text":"Open Sunday?","items":[{"answer_text":"Yes"}]},{"question_text":"Parking?","items":null}]}`,
			reflect.TypeFor[*seo.BusinessQuestionsResult](),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != tt.path || r.URL.RawQuery != "" {
					t.Errorf("request = %s %s", r.Method, r.URL)
				}
				raw, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				if tt.method == http.MethodGet {
					if len(raw) != 0 {
						t.Errorf("GET body = %s", raw)
					}
				} else {
					businessToolJSON(t, json.RawMessage(raw), tt.task)
				}
				_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"result":` + tt.response + `}]}`))
			}))
			t.Cleanup(server.Close)
			api := dataforseo.New("synthetic-test-key")
			api.BaseURL = server.URL
			got, err := New(seo.New(api)).Call(context.Background(), tt.name, json.RawMessage(tt.args))
			if err != nil {
				t.Fatal(err)
			}
			if reflect.TypeOf(got) != tt.resultType {
				t.Fatalf("result type = %T", got)
			}
			businessToolJSON(t, got, tt.want)
		})
	}
}

func TestBusinessToolArgumentErrors(t *testing.T) {
	for _, tt := range []struct{ name, args string }{
		{"search_local_businesses", `null`},
		{"search_local_businesses", `{"near":{"latitude":0,"radiusKm":1}}`},
		{"search_local_businesses", `{"near":{"latitude":0,"longitude":0,"radiusKm":1},"limit":0}`},
		{"search_local_businesses", `{"near":{"latitude":0,"longitude":0,"radiusKm":1},"limit":1.5}`},
		{"search_local_businesses", `{"near":{"latitude":0,"longitude":0,"radiusKm":1},"minReviews":2.5}`},
		{"search_local_businesses", `{"near":{"latitude":0,"longitude":0,"radiusKm":1},"categories":[]}`},
		{"search_local_businesses", `{"near":{"latitude":0,"longitude":0,"radiusKm":1},"query":""}`},
		{"list_business_categories", `{"limit":201}`},
		{"list_business_categories", `{"query":""}`},
		{"list_business_categories", `{"limit":`},
		{"get_business_profile", `{}`},
		{"get_business_profile", `{"businessName":"Cafe","cid":"123"}`},
		{"get_business_profile", `{"cid":"","placeId":"place"}`},
		{"get_business_profile", `{"cid":123}`},
		{"get_business_profile", `{"cid":"123","near":{"latitude":0,"longitude":0,"radiusKm":0}}`},
		{"get_google_business_questions", `{"cid":"123"}`},
		{"get_google_business_questions", `{"cid":"123","near":{"latitude":0,"longitude":0,"radiusKm":1},"depth":101}`},
		{"get_google_business_questions", `{"cid":"123","near":{"latitude":0,"longitude":0,"radiusKm":1},"depth":1.5}`},
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
