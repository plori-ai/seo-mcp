package seo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

func businessTestNear(radius float64) *BusinessSearchNear {
	return &BusinessSearchNear{Latitude: domainTestPtr(33.123456789), Longitude: domainTestPtr(-84.987654321), RadiusKM: radius}
}

func businessTestCall(c *Client, req any) (any, error) {
	switch req := req.(type) {
	case SearchLocalBusinessesRequest:
		return c.SearchLocalBusinesses(context.Background(), req)
	case ListBusinessCategoriesRequest:
		return c.ListBusinessCategories(context.Background(), req)
	case BusinessProfileRequest:
		return c.BusinessProfile(context.Background(), req)
	case BusinessQuestionsRequest:
		return c.BusinessQuestions(context.Background(), req)
	default:
		panic(fmt.Sprintf("unexpected request type %T", req))
	}
}

func TestSearchLocalBusinessesRequestsAndShape(t *testing.T) {
	response := domainTestFixture(t, "testdata/business/search_result.json")
	want := domainTestFixture(t, "testdata/business/search_want.json")
	for _, tt := range []struct {
		name string
		req  SearchLocalBusinessesRequest
		task string
	}{
		{"defaults and rounded radius", SearchLocalBusinessesRequest{Near: businessTestNear(1.5)}, `{"location_coordinate":"33.1234568,-84.9876543,2","limit":20}`},
		{"all filters and full limit", SearchLocalBusinessesRequest{
			Query: domainTestPtr(" Example Cafe "), Near: businessTestNear(5.4), Categories: []string{"cafe", "bakery"},
			MinRating: domainTestPtr(4.5), MinReviews: domainTestPtr(0), IsClaimed: domainTestPtr(false), SortBy: "rating", Limit: domainTestPtr(50), Offset: domainTestPtr(1000),
		}, `{"title":" Example Cafe ","location_coordinate":"33.1234568,-84.9876543,5","categories":["cafe","bakery"],"filters":[["rating.value",">=",4.5],"and",["rating.votes_count",">=",0]],"is_claimed":false,"order_by":["rating.value,desc"],"limit":50,"offset":1000}`},
		{"reviews order and one filter", SearchLocalBusinessesRequest{Near: businessTestNear(1), MinReviews: domainTestPtr(15), IsClaimed: domainTestPtr(true), SortBy: "reviews", Limit: domainTestPtr(1), Offset: domainTestPtr(0)}, `{"location_coordinate":"33.1234568,-84.9876543,1","filters":[["rating.votes_count",">=",15]],"is_claimed":true,"order_by":["rating.votes_count,desc"],"limit":1,"offset":0}`},
		{"rating only", SearchLocalBusinessesRequest{Near: businessTestNear(1), MinRating: domainTestPtr(1.0)}, `{"location_coordinate":"33.1234568,-84.9876543,1","limit":20,"filters":[["rating.value",">=",1]]}`},
		{"explicit relevance and radius maximum", SearchLocalBusinessesRequest{Near: &BusinessSearchNear{Latitude: domainTestPtr(0.0), Longitude: domainTestPtr(0.0), RadiusKM: 100000}, SortBy: "relevance", MinRating: domainTestPtr(5.0)}, `{"location_coordinate":"0,0,100000","limit":20,"filters":[["rating.value",">=",5]]}`},
		{"coordinate bounds", SearchLocalBusinessesRequest{Near: &BusinessSearchNear{Latitude: domainTestPtr(-90.0), Longitude: domainTestPtr(180.0), RadiusKM: 1}}, `{"location_coordinate":"-90,180,1","limit":20}`},
		{"coordinate ties round away from zero", SearchLocalBusinessesRequest{Near: &BusinessSearchNear{Latitude: domainTestPtr(0.00390625), Longitude: domainTestPtr(-0.00390625), RadiusKM: 1}}, `{"location_coordinate":"0.0039063,-0.0039063,1","limit":20}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v3/business_data/business_listings/search/live" {
					t.Errorf("path = %s", r.URL.Path)
				}
				domainTestBody(t, r, tt.task)
				domainTestResponse(w, 20000, "Ok.", response)
			}, WithDefaultMarket(Market{LocationCode: 2276, LanguageCode: "de"}))
			got, err := c.SearchLocalBusinesses(context.Background(), tt.req)
			if err != nil {
				t.Fatal(err)
			}
			domainTestJSON(t, got, want)
			if calls.Load() != 1 {
				t.Errorf("calls = %d", calls.Load())
			}
		})
	}
}

func TestListBusinessCategoriesRequestsAndShape(t *testing.T) {
	response := domainTestFixture(t, "testdata/business/categories_result.json")
	for _, tt := range []struct {
		name string
		req  ListBusinessCategoriesRequest
		want string
	}{
		{"default sorted with stable ties and missing counts", ListBusinessCategoriesRequest{}, `{"categories":[{"category":"Cafe","businessCount":120},{"category":"PLUMBER","businessCount":90},{"category":"equal_count","businessCount":90},{"category":"plumbing_supply_store","businessCount":40},{"category":"missing_count","businessCount":null},{"category":"null_count","businessCount":null},{"category":"zero_count","businessCount":0}]}`},
		{"case insensitive filter then rank then limit", ListBusinessCategoriesRequest{Query: domainTestPtr("PlUm"), Limit: domainTestPtr(1)}, `{"categories":[{"category":"PLUMBER","businessCount":90}]}`},
		{"no matches", ListBusinessCategoriesRequest{Query: domainTestPtr("no_such_category")}, `{"categories":[]}`},
		{"query is not trimmed", ListBusinessCategoriesRequest{Query: domainTestPtr(" Cafe ")}, `{"categories":[]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != "/v3/business_data/business_listings/categories" || r.URL.RawQuery != "" {
					t.Errorf("request = %s %s", r.Method, r.URL)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil || len(body) != 0 {
					t.Errorf("GET body = %q, %v", body, err)
				}
				domainTestResponse(w, 20000, "Ok.", response)
			})
			for range 2 {
				got, err := c.ListBusinessCategories(context.Background(), tt.req)
				if err != nil {
					t.Fatal(err)
				}
				domainTestJSON(t, got, tt.want)
			}
			if calls.Load() != 2 {
				t.Errorf("calls = %d; each call must fetch again", calls.Load())
			}
		})
	}
}

func TestListBusinessCategoriesLimits(t *testing.T) {
	rows := make([]map[string]any, 205)
	for i := range rows {
		rows[i] = map[string]any{"category_name": fmt.Sprintf("category_%03d", i), "business_count": i}
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) { domainTestResponse(w, 20000, "Ok.", string(raw)) })
	for _, tt := range []struct {
		limit *int
		count int
	}{{nil, 50}, {domainTestPtr(200), 200}} {
		got, err := c.ListBusinessCategories(context.Background(), ListBusinessCategoriesRequest{Limit: tt.limit})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Categories) != tt.count || got.Categories[0].Category != "category_204" || got.Categories[tt.count-1].Category != fmt.Sprintf("category_%03d", 205-tt.count) {
			t.Errorf("unexpected page: %+v", got)
		}
	}
}

func TestBusinessProfileRequestsAndShape(t *testing.T) {
	response := domainTestFixture(t, "testdata/business/profile_result.json")
	want := domainTestFixture(t, "testdata/business/profile_want.json")
	for _, tt := range []struct {
		name   string
		req    BusinessProfileRequest
		market *Market
		task   string
	}{
		{"default market and name", BusinessProfileRequest{BusinessName: domainTestPtr("Example Cafe")}, nil, `{"keyword":"Example Cafe","location_code":2840,"language_code":"en"}`},
		{"client default market", BusinessProfileRequest{CID: domainTestPtr("123")}, &Market{LocationCode: 2276, LanguageCode: "de"}, `{"keyword":"cid:123","location_code":2276,"language_code":"de"}`},
		{"explicit location preserves default language", BusinessProfileRequest{PlaceID: domainTestPtr("place"), LocationCode: 2276}, nil, `{"keyword":"place_id:place","location_code":2276,"language_code":"en"}`},
		{"explicit city location", BusinessProfileRequest{PlaceID: domainTestPtr("place"), LocationCode: 1023191}, nil, `{"keyword":"place_id:place","location_code":1023191,"language_code":"en"}`},
		{"explicit language independent of default country", BusinessProfileRequest{CID: domainTestPtr("123"), LanguageCode: "de"}, nil, `{"keyword":"cid:123","location_code":2840,"language_code":"de"}`},
		{"explicit location and language", BusinessProfileRequest{CID: domainTestPtr("123"), LocationCode: 2276, LanguageCode: "es"}, nil, `{"keyword":"cid:123","location_code":2276,"language_code":"es"}`},
		{"explicit language", BusinessProfileRequest{BusinessName: domainTestPtr("Cafe"), LocationCode: 2840, LanguageCode: "es"}, nil, `{"keyword":"Cafe","location_code":2840,"language_code":"es"}`},
		{"ads only country is accepted", BusinessProfileRequest{CID: domainTestPtr("123"), LocationCode: 2352, LanguageCode: "en"}, nil, `{"keyword":"cid:123","location_code":2352,"language_code":"en"}`},
		{"coordinate ignores location and defaults radius", BusinessProfileRequest{CID: domainTestPtr("123"), LocationCode: 999999, Near: &BusinessProfileNear{Latitude: domainTestPtr(33.123456789), Longitude: domainTestPtr(-84.987654321)}}, nil, `{"keyword":"cid:123","location_coordinate":"33.1234568,-84.9876543,10000","language_code":"en"}`},
		{"minimum radius and zero coordinates", BusinessProfileRequest{PlaceID: domainTestPtr("place"), Near: &BusinessProfileNear{Latitude: domainTestPtr(0.0), Longitude: domainTestPtr(math.Copysign(0, -1)), RadiusKM: domainTestPtr(0.2)}}, nil, `{"keyword":"place_id:place","location_coordinate":"0,0,200","language_code":"en"}`},
		{"meter rounding and coordinate notation", BusinessProfileRequest{BusinessName: domainTestPtr("Cafe"), Near: &BusinessProfileNear{Latitude: domainTestPtr(0.0000001), Longitude: domainTestPtr(-0.00000001), RadiusKM: domainTestPtr(1.2345)}, LanguageCode: "es"}, nil, `{"keyword":"Cafe","location_coordinate":"1e-7,0,1235","language_code":"es"}`},
		{"maximum radius and coordinate bounds", BusinessProfileRequest{BusinessName: domainTestPtr("Cafe"), Near: &BusinessProfileNear{Latitude: domainTestPtr(90.0), Longitude: domainTestPtr(-180.0), RadiusKM: domainTestPtr(199.0)}}, nil, `{"keyword":"Cafe","location_coordinate":"90,-180,199000","language_code":"en"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.market != nil {
				opts = append(opts, WithDefaultMarket(*tt.market))
			}
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v3/business_data/google/my_business_info/live" {
					t.Errorf("path = %s", r.URL.Path)
				}
				domainTestBody(t, r, tt.task)
				domainTestResponse(w, 20000, "Ok.", response)
			}, opts...)
			got, err := c.BusinessProfile(context.Background(), tt.req)
			if err != nil {
				t.Fatal(err)
			}
			domainTestJSON(t, got, want)
			if string(got.Profile["cid"]) != "9007199254740993" {
				t.Errorf("provider number changed: %s", got.Profile["cid"])
			}
		})
	}
}

func TestBusinessProfileFallbacks(t *testing.T) {
	for _, tt := range []struct{ name, response, want string }{
		{"missing item url", `[{"check_url":"entry","items":[{"title":"Cafe"}]}]`, `{"profile":{"title":"Cafe","check_url":"entry"}}`},
		{"null item url", `[{"check_url":"entry","items":[{"check_url":null}]}]`, `{"profile":{"check_url":"entry"}}`},
		{"item url takes precedence", `[{"check_url":"entry","items":[{"check_url":"item"}]}]`, `{"profile":{"check_url":"item"}}`},
		{"empty item url is preserved", `[{"check_url":"entry","items":[{"check_url":""}]}]`, `{"profile":{"check_url":""}}`},
		{"no fallback", `[{"items":[{"check_url":null}]}]`, `{"profile":{}}`},
		{"null fallback", `[{"check_url":null,"items":[{}]}]`, `{"profile":{"check_url":null}}`},
		{"missing urls", `[{"items":[{}]}]`, `{"profile":{}}`},
		{"null first item", `[{"items":[null,{"title":"ignored"}]}]`, `{"profile":null}`},
		{"scalar first item", `[{"items":[42]}]`, `{"profile":null}`},
		{"array first item", `[{"items":[[]]}]`, `{"profile":null}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) { domainTestResponse(w, 20000, "Ok.", tt.response) })
			got, err := c.BusinessProfile(context.Background(), BusinessProfileRequest{CID: domainTestPtr("123")})
			if err != nil {
				t.Fatal(err)
			}
			domainTestJSON(t, got, tt.want)
		})
	}
}

func TestBusinessQuestionsRequestsAndShape(t *testing.T) {
	response := domainTestFixture(t, "testdata/business/questions_result.json")
	want := domainTestFixture(t, "testdata/business/questions_want.json")
	for _, tt := range []struct {
		name   string
		req    BusinessQuestionsRequest
		market *Market
		task   string
	}{
		{"name and defaults", BusinessQuestionsRequest{BusinessName: domainTestPtr("Cafe"), Near: businessTestNear(1)}, nil, `{"keyword":"Cafe","location_coordinate":"33.1234568,-84.9876543,1000","language_code":"en","depth":20}`},
		{"cid full depth and clamped radius", BusinessQuestionsRequest{CID: domainTestPtr("123"), Near: businessTestNear(100000), Depth: domainTestPtr(100), LanguageCode: "es"}, nil, `{"keyword":"cid:123","location_coordinate":"33.1234568,-84.9876543,199999","language_code":"es","depth":100}`},
		{"place id and custom default market", BusinessQuestionsRequest{PlaceID: domainTestPtr("place"), Near: businessTestNear(1.2345), Depth: domainTestPtr(1)}, &Market{LocationCode: 2276, LanguageCode: "de"}, `{"keyword":"place_id:place","location_coordinate":"33.1234568,-84.9876543,1235","language_code":"de","depth":1}`},
		{"rounding reaches clamp", BusinessQuestionsRequest{CID: domainTestPtr("123"), Near: businessTestNear(199.9996)}, nil, `{"keyword":"cid:123","location_coordinate":"33.1234568,-84.9876543,199999","language_code":"en","depth":20}`},
		{"explicit language independent of default country", BusinessQuestionsRequest{CID: domainTestPtr("123"), Near: businessTestNear(1), LanguageCode: "de"}, nil, `{"keyword":"cid:123","location_coordinate":"33.1234568,-84.9876543,1000","language_code":"de","depth":20}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.market != nil {
				opts = append(opts, WithDefaultMarket(*tt.market))
			}
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v3/business_data/google/questions_and_answers/live" {
					t.Errorf("path = %s", r.URL.Path)
				}
				domainTestBody(t, r, tt.task)
				domainTestResponse(w, 20000, "Ok.", response)
			}, opts...)
			got, err := c.BusinessQuestions(context.Background(), tt.req)
			if err != nil {
				t.Fatal(err)
			}
			domainTestJSON(t, got, want)
		})
	}
}

func TestBusinessEmptyAndTaskErrors(t *testing.T) {
	for _, operation := range []struct {
		name           string
		req            any
		empty          string
		noResultsEmpty bool
	}{
		{"search", SearchLocalBusinessesRequest{Near: businessTestNear(1)}, `{"businesses":[]}`, true},
		{"categories", ListBusinessCategoriesRequest{}, `{"categories":[]}`, false},
		{"profile", BusinessProfileRequest{CID: domainTestPtr("123")}, `{"profile":null}`, true},
		{"questions", BusinessQuestionsRequest{CID: domainTestPtr("123"), Near: businessTestNear(1)}, `{"questions":[]}`, true},
	} {
		for _, tt := range []struct {
			name            string
			status          int
			message, result string
		}{
			{"no result", 20000, "Ok.", `[]`},
			{"null result", 20000, "Ok.", `null`},
			{"null first result", 20000, "Ok.", `[null]`},
			{"missing items", 20000, "Ok.", `[{}]`},
			{"null items", 20000, "Ok.", `[{"items":null,"items_without_answers":null}]`},
			{"empty items", 20000, "Ok.", `[{"items":[],"items_without_answers":[]}]`},
			{"no search results", 40501, "No Search Results.", `null`},
			{"invalid field", 40501, "Invalid Field: 'keyword'.", `null`},
			{"provider error", 40101, "Internal SE Server Error.", `null`},
		} {
			t.Run(operation.name+"/"+tt.name, func(t *testing.T) {
				c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) { domainTestResponse(w, tt.status, tt.message, tt.result) })
				got, err := businessTestCall(c, operation.req)
				if tt.status == 20000 || (tt.name == "no search results" && operation.noResultsEmpty) {
					if err != nil {
						t.Fatal(err)
					}
					domainTestJSON(t, got, operation.empty)
				} else {
					var apiErr *dataforseo.Error
					if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
						t.Fatalf("want task error %d, got %v", tt.status, err)
					}
				}
			})
		}
	}
}

func TestBusinessInputErrors(t *testing.T) {
	near := businessTestNear(1)
	for _, tt := range []struct {
		name string
		req  any
	}{
		{"search missing near", SearchLocalBusinessesRequest{}},
		{"search missing latitude", SearchLocalBusinessesRequest{Near: &BusinessSearchNear{Longitude: domainTestPtr(0.0), RadiusKM: 1}}},
		{"search missing longitude", SearchLocalBusinessesRequest{Near: &BusinessSearchNear{Latitude: domainTestPtr(0.0), RadiusKM: 1}}},
		{"search latitude range", SearchLocalBusinessesRequest{Near: &BusinessSearchNear{Latitude: domainTestPtr(90.1), Longitude: domainTestPtr(0.0), RadiusKM: 1}}},
		{"search longitude range", SearchLocalBusinessesRequest{Near: &BusinessSearchNear{Latitude: domainTestPtr(0.0), Longitude: domainTestPtr(-180.1), RadiusKM: 1}}},
		{"search latitude nan", SearchLocalBusinessesRequest{Near: &BusinessSearchNear{Latitude: domainTestPtr(math.NaN()), Longitude: domainTestPtr(0.0), RadiusKM: 1}}},
		{"search radius minimum", SearchLocalBusinessesRequest{Near: businessTestNear(0.9)}},
		{"search radius maximum", SearchLocalBusinessesRequest{Near: businessTestNear(100001)}},
		{"search radius infinity", SearchLocalBusinessesRequest{Near: businessTestNear(math.Inf(1))}},
		{"search empty query", SearchLocalBusinessesRequest{Near: near, Query: domainTestPtr("")}},
		{"search long query", SearchLocalBusinessesRequest{Near: near, Query: domainTestPtr(strings.Repeat("x", 201))}},
		{"search utf16 query length", SearchLocalBusinessesRequest{Near: near, Query: domainTestPtr(strings.Repeat("😀", 101))}},
		{"search empty categories", SearchLocalBusinessesRequest{Near: near, Categories: []string{}}},
		{"search too many categories", SearchLocalBusinessesRequest{Near: near, Categories: strings.Split(strings.Repeat("a,", 10)+"a", ",")}},
		{"search empty category", SearchLocalBusinessesRequest{Near: near, Categories: []string{""}}},
		{"search long category", SearchLocalBusinessesRequest{Near: near, Categories: []string{strings.Repeat("x", 121)}}},
		{"search rating minimum", SearchLocalBusinessesRequest{Near: near, MinRating: domainTestPtr(0.9)}},
		{"search rating maximum", SearchLocalBusinessesRequest{Near: near, MinRating: domainTestPtr(5.1)}},
		{"search rating nan", SearchLocalBusinessesRequest{Near: near, MinRating: domainTestPtr(math.NaN())}},
		{"search negative reviews", SearchLocalBusinessesRequest{Near: near, MinReviews: domainTestPtr(-1)}},
		{"search bad sort", SearchLocalBusinessesRequest{Near: near, SortBy: "name"}},
		{"search zero limit", SearchLocalBusinessesRequest{Near: near, Limit: domainTestPtr(0)}},
		{"search large limit", SearchLocalBusinessesRequest{Near: near, Limit: domainTestPtr(51)}},
		{"search negative offset", SearchLocalBusinessesRequest{Near: near, Offset: domainTestPtr(-1)}},
		{"search large offset", SearchLocalBusinessesRequest{Near: near, Offset: domainTestPtr(1001)}},
		{"categories empty query", ListBusinessCategoriesRequest{Query: domainTestPtr("")}},
		{"categories long query", ListBusinessCategoriesRequest{Query: domainTestPtr(strings.Repeat("x", 81))}},
		{"categories zero limit", ListBusinessCategoriesRequest{Limit: domainTestPtr(0)}},
		{"categories large limit", ListBusinessCategoriesRequest{Limit: domainTestPtr(201)}},
		{"profile missing identifier", BusinessProfileRequest{}},
		{"profile two identifiers", BusinessProfileRequest{BusinessName: domainTestPtr("Cafe"), CID: domainTestPtr("123")}},
		{"profile empty identifier", BusinessProfileRequest{CID: domainTestPtr("")}},
		{"profile long name", BusinessProfileRequest{BusinessName: domainTestPtr(strings.Repeat("x", 201))}},
		{"profile long cid", BusinessProfileRequest{CID: domainTestPtr(strings.Repeat("x", 65))}},
		{"profile long place id", BusinessProfileRequest{PlaceID: domainTestPtr(strings.Repeat("x", 257))}},
		{"profile negative location", BusinessProfileRequest{CID: domainTestPtr("123"), LocationCode: -1}},
		{"profile unsupported language", BusinessProfileRequest{CID: domainTestPtr("123"), LanguageCode: "zz"}},
		{"profile empty near", BusinessProfileRequest{CID: domainTestPtr("123"), Near: &BusinessProfileNear{}}},
		{"profile small radius", BusinessProfileRequest{CID: domainTestPtr("123"), Near: &BusinessProfileNear{Latitude: near.Latitude, Longitude: near.Longitude, RadiusKM: domainTestPtr(0.19)}}},
		{"profile large radius", BusinessProfileRequest{CID: domainTestPtr("123"), Near: &BusinessProfileNear{Latitude: near.Latitude, Longitude: near.Longitude, RadiusKM: domainTestPtr(199.1)}}},
		{"profile nan radius", BusinessProfileRequest{CID: domainTestPtr("123"), Near: &BusinessProfileNear{Latitude: near.Latitude, Longitude: near.Longitude, RadiusKM: domainTestPtr(math.NaN())}}},
		{"questions missing identifier", BusinessQuestionsRequest{Near: near}},
		{"questions two identifiers", BusinessQuestionsRequest{CID: domainTestPtr("123"), PlaceID: domainTestPtr("place"), Near: near}},
		{"questions empty identifier", BusinessQuestionsRequest{BusinessName: domainTestPtr(""), Near: near}},
		{"questions missing near", BusinessQuestionsRequest{CID: domainTestPtr("123")}},
		{"questions small radius", BusinessQuestionsRequest{CID: domainTestPtr("123"), Near: businessTestNear(0.2)}},
		{"questions large radius", BusinessQuestionsRequest{CID: domainTestPtr("123"), Near: businessTestNear(100001)}},
		{"questions zero depth", BusinessQuestionsRequest{CID: domainTestPtr("123"), Near: near, Depth: domainTestPtr(0)}},
		{"questions large depth", BusinessQuestionsRequest{CID: domainTestPtr("123"), Near: near, Depth: domainTestPtr(101)}},
		{"questions unsupported language", BusinessQuestionsRequest{CID: domainTestPtr("123"), Near: near, LanguageCode: "zz"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := businessTestCall(New(nil), tt.req)
			var inputErr *InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("want InputError, got %v", err)
			}
		})
	}
}
