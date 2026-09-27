package seo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

func TestLocalSerpResultsRequests(t *testing.T) {
	fixture := domainTestFixture(t, "testdata/local_serp/snapshot.json")
	want := domainTestFixture(t, "testdata/local_serp/snapshot_want.json")
	for _, tt := range []struct {
		name, args, path, task string
		opts                   []Option
	}{
		{"default", `{"keyword":"Coffee Near Me","near":{"latitude":40,"longitude":-74}}`, pathLocalSerpMaps, `{"keyword":"Coffee Near Me","location_coordinate":"40,-74","language_code":"en","device":"mobile","os":"android","depth":20,"search_places":false}`, nil},
		{"client market", `{"keyword":"coffee","near":{"latitude":40,"longitude":-74}}`, pathLocalSerpMaps, `{"keyword":"coffee","location_coordinate":"40,-74","language_code":"de","device":"mobile","os":"android","depth":20,"search_places":false}`, []Option{WithDefaultMarket(Market{2276, "de"})}},
		{"explicit language independent of country", `{"keyword":"café","near":{"latitude":48.85661401,"longitude":2.35222194,"zoom":18},"languageCode":"fr","device":"desktop","depth":100}`, pathLocalSerpMaps, `{"keyword":"café","location_coordinate":"48.856614,2.3522219,18z","language_code":"fr","device":"desktop","os":"windows","depth":100,"search_places":false}`, nil},
		{"finder desktop minimum depth", `{"keyword":"coffee","near":{"latitude":0,"longitude":0,"zoom":4},"searchType":"local_finder","device":"desktop","depth":1}`, pathLocalSerpFinder, `{"keyword":"coffee","location_coordinate":"0,0,4z","language_code":"en","device":"desktop","os":"windows","depth":1}`, nil},
		{"finder mobile", `{"keyword":"coffee","near":{"latitude":-90,"longitude":180},"searchType":"local_finder"}`, pathLocalSerpFinder, `{"keyword":"coffee","location_coordinate":"-90,180","language_code":"en","device":"mobile","os":"android","depth":20}`, nil},
		{"ads-only default market", `{"keyword":"coffee","near":{"latitude":64,"longitude":-21}}`, pathLocalSerpMaps, `{"keyword":"coffee","location_coordinate":"64,-21","language_code":"en","device":"mobile","os":"android","depth":20,"search_places":false}`, []Option{WithDefaultMarket(Market{2352, "en"})}},
		{"coordinate decimal ties", `{"keyword":" coffee ","near":{"latitude":0.00390625,"longitude":-0.00390625}}`, pathLocalSerpMaps, `{"keyword":" coffee ","location_coordinate":"0.0039063,-0.0039063","language_code":"en","device":"mobile","os":"android","depth":20,"search_places":false}`, nil},
		{"coordinate exponent and negative zero", `{"keyword":"coffee","near":{"latitude":0.0000001,"longitude":-0.00000001}}`, pathLocalSerpMaps, `{"keyword":"coffee","location_coordinate":"1e-7,0","language_code":"en","device":"mobile","os":"android","depth":20,"search_places":false}`, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newKWClient(t, func(string, map[string]any) kwReply {
				return kwReply{Result: json.RawMessage(fixture)}
			}, tt.opts...)
			var req LocalSerpResultsRequest
			if err := json.Unmarshal([]byte(tt.args), &req); err != nil {
				t.Fatal(err)
			}
			result, err := c.LocalSerpResults(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			domainTestJSON(t, result, want)
			calls := recorder.all()
			if len(calls) != 1 {
				t.Fatalf("calls = %d, want 1", len(calls))
			}
			kwAssertTask(t, calls[0], tt.path, tt.task)
		})
	}
}

func TestLocalSerpResultsEmptyAndErrors(t *testing.T) {
	for _, searchType := range []string{"maps", "local_finder"} {
		for _, tt := range []struct {
			name, result, message string
			status                int
			wantError             bool
		}{
			{"null result", "null", "", 20000, false},
			{"empty result", "[]", "", 20000, false},
			{"null first result", "[null]", "", 20000, false},
			{"missing items", "[{}]", "", 20000, false},
			{"null items", `[{"items":null}]`, "", 20000, false},
			{"empty items", `[{"items":[]}]`, "", 20000, false},
			{"no results 40501", "null", "No Search Results.", 40501, false},
			{"no results 40102", "null", "No Search Results.", 40102, false},
			{"same code invalid field", "null", "Invalid Field: 'location_coordinate'.", 40501, true},
			{"task error", "null", "Internal SE Server Error.", 40101, true},
			{"invalid result", `[{"items":{}}]`, "", 20000, true},
		} {
			t.Run(searchType+"/"+tt.name, func(t *testing.T) {
				c, recorder := newKWClient(t, func(string, map[string]any) kwReply {
					return kwReply{Status: tt.status, Message: tt.message, Result: json.RawMessage(tt.result)}
				})
				result, err := c.LocalSerpResults(context.Background(), LocalSerpResultsRequest{
					Keyword: "coffee", SearchType: searchType,
					Near: &LocalSerpNear{Latitude: domainTestPtr(0.0), Longitude: domainTestPtr(0.0)},
				})
				if tt.wantError {
					if err == nil || result != nil {
						t.Fatalf("result = %+v, error = %v", result, err)
					}
					if tt.status != 20000 {
						var provider *dataforseo.Error
						if !errors.As(err, &provider) || provider.StatusCode != tt.status {
							t.Fatalf("error = %v", err)
						}
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					domainTestJSON(t, result, `{"results":[]}`)
				}
				calls := recorder.all()
				if len(calls) != 1 {
					t.Fatalf("calls = %d", len(calls))
				}
				path, suffix := pathLocalSerpMaps, `,"search_places":false`
				if searchType == "local_finder" {
					path, suffix = pathLocalSerpFinder, ""
				}
				kwAssertTask(t, calls[0], path, `{"keyword":"coffee","location_coordinate":"0,0","language_code":"en","device":"mobile","os":"android","depth":20`+suffix+`}`)
			})
		}
	}
}

func TestLocalSerpInputErrors(t *testing.T) {
	for _, tt := range []struct{ name, args string }{
		{"keyword missing", `{"near":{"latitude":0,"longitude":0}}`},
		{"keyword too long", `{"keyword":"` + strings.Repeat("a", 121) + `","near":{"latitude":0,"longitude":0}}`},
		{"keyword UTF16 bound", `{"keyword":"` + strings.Repeat("😀", 61) + `","near":{"latitude":0,"longitude":0}}`},
		{"near missing", `{"keyword":"coffee"}`},
		{"latitude missing", `{"keyword":"coffee","near":{"longitude":0}}`},
		{"longitude missing", `{"keyword":"coffee","near":{"latitude":0}}`},
		{"latitude low", `{"keyword":"coffee","near":{"latitude":-90.1,"longitude":0}}`},
		{"latitude high", `{"keyword":"coffee","near":{"latitude":90.1,"longitude":0}}`},
		{"longitude low", `{"keyword":"coffee","near":{"latitude":0,"longitude":-180.1}}`},
		{"longitude high", `{"keyword":"coffee","near":{"latitude":0,"longitude":180.1}}`},
		{"zoom low", `{"keyword":"coffee","near":{"latitude":0,"longitude":0,"zoom":3}}`},
		{"zoom high", `{"keyword":"coffee","near":{"latitude":0,"longitude":0,"zoom":19}}`},
		{"depth zero", `{"keyword":"coffee","near":{"latitude":0,"longitude":0},"depth":0}`},
		{"depth high", `{"keyword":"coffee","near":{"latitude":0,"longitude":0},"depth":101}`},
		{"search type", `{"keyword":"coffee","near":{"latitude":0,"longitude":0},"searchType":"organic"}`},
		{"device", `{"keyword":"coffee","near":{"latitude":0,"longitude":0},"device":"tablet"}`},
		{"language", `{"keyword":"coffee","near":{"latitude":0,"longitude":0},"languageCode":"xx"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var req LocalSerpResultsRequest
			if err := json.Unmarshal([]byte(tt.args), &req); err != nil {
				t.Fatal(err)
			}
			_, err := New(nil).LocalSerpResults(context.Background(), req)
			var inputErr *InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("error = %v, want InputError", err)
			}
		})
	}
}

func TestLocalRankGridFixture(t *testing.T) {
	var fixtures []struct {
		Coordinate string          `json:"coordinate"`
		Status     int             `json:"status"`
		Message    string          `json:"message"`
		Result     json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(domainTestFixture(t, "testdata/local_serp/grid_responses.json")), &fixtures); err != nil {
		t.Fatal(err)
	}
	c, recorder := newKWClient(t, func(path string, task map[string]any) kwReply {
		for _, fixture := range fixtures {
			if task["location_coordinate"] == fixture.Coordinate {
				return kwReply{Status: fixture.Status, Message: fixture.Message, Result: fixture.Result}
			}
		}
		t.Errorf("unexpected request %s: %v", path, task)
		return kwReply{Status: 50000, Message: "Unexpected coordinate"}
	})
	result, err := c.LocalRankGrid(context.Background(), LocalRankGridRequest{
		Keyword: "coffee", Target: &LocalRankGridTarget{CID: domainTestPtr("123")},
		Center: &LocalRankGridCenter{Latitude: domainTestPtr(40.0), Longitude: domainTestPtr(-74.0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	domainTestJSON(t, result, domainTestFixture(t, "testdata/local_serp/grid_want.json"))
	calls := recorder.all()
	if len(calls) != 9 {
		t.Fatalf("calls = %d", len(calls))
	}
	seen := map[string]bool{}
	for _, call := range calls {
		coordinate := call.Task["location_coordinate"].(string)
		if seen[coordinate] {
			t.Errorf("duplicate coordinate %s", coordinate)
		}
		seen[coordinate] = true
		kwAssertTask(t, call, pathLocalSerpMaps, fmt.Sprintf(`{"keyword":"coffee","location_coordinate":%q,"language_code":"en","device":"mobile","os":"android","depth":20,"search_places":false}`, coordinate))
	}
}

func TestLocalRankGridRequests(t *testing.T) {
	for _, tt := range []struct {
		name, language, device, os, expectedLanguage string
		size                                         int
		spacing, latitude                            float64
		zoom                                         *int
		expectedZoom                                 int
		opts                                         []Option
	}{
		{"default market", "", "", "android", "en", 3, 2, 40, nil, 13, nil},
		{"client market", "", "", "android", "de", 3, 2, 40, nil, 13, []Option{WithDefaultMarket(Market{2276, "de"})}},
		{"five wide explicit options", "fr", "desktop", "windows", "fr", 5, 2, 40, domainTestPtr(18), 18, nil},
		{"minimum spacing", "", "", "android", "en", 3, 0.25, 0, nil, 16, nil},
		{"maximum spacing", "", "", "android", "en", 3, 10, 40, nil, 10, nil},
		{"polar cosine and zoom floor", "", "", "android", "en", 3, 10, 90, nil, 4, nil},
		{"explicit minimum zoom", "", "mobile", "android", "en", 3, 2, -90, domainTestPtr(4), 4, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newKWClient(t, func(string, map[string]any) kwReply { return kwReply{Result: json.RawMessage(`[{"items":[]}]`)} }, tt.opts...)
			result, err := c.LocalRankGrid(context.Background(), LocalRankGridRequest{
				Keyword: "coffee", Target: &LocalRankGridTarget{CID: domainTestPtr("123")},
				Center:   &LocalRankGridCenter{Latitude: &tt.latitude, Longitude: domainTestPtr(-74.0)},
				GridSize: &tt.size, SpacingKm: &tt.spacing, Zoom: tt.zoom, Device: tt.device, LanguageCode: tt.language,
			})
			if err != nil {
				t.Fatal(err)
			}
			calls := recorder.all()
			if len(calls) != tt.size*tt.size || len(result.Grid) != len(calls) {
				t.Fatalf("calls = %d, grid length = %d", len(calls), len(result.Grid))
			}
			device := tt.device
			if device == "" {
				device = "mobile"
			}
			seen := map[string]bool{}
			for _, call := range calls {
				coordinate := call.Task["location_coordinate"].(string)
				if seen[coordinate] || !strings.HasSuffix(coordinate, fmt.Sprintf(",%dz", tt.expectedZoom)) {
					t.Errorf("unexpected/duplicate coordinate %q", coordinate)
				}
				seen[coordinate] = true
				kwAssertTask(t, call, pathLocalSerpMaps, fmt.Sprintf(`{"keyword":"coffee","location_coordinate":%q,"language_code":%q,"device":%q,"os":%q,"depth":20,"search_places":false}`, coordinate, tt.expectedLanguage, device, tt.os))
			}
			if tt.size == 5 {
				for _, latitude := range []string{"40.0361749", "40.0180874", "40", "39.9819126", "39.9638251"} {
					for _, longitude := range []string{"-74.0469065", "-74.0234532", "-74", "-73.9765468", "-73.9530935"} {
						if !seen[latitude+","+longitude+",18z"] {
							t.Errorf("missing coordinate %s,%s,18z", latitude, longitude)
						}
					}
				}
			}
			for i, point := range result.Grid {
				if point.Row != i/tt.size || point.Col != i%tt.size || point.Error || point.Rank != nil || point.TopResult != nil || point.ResultsCount == nil || *point.ResultsCount != 0 {
					t.Errorf("point %d = %+v", i, point)
				}
			}
			domainTestJSON(t, result.Summary, fmt.Sprintf(`{"pointsSearched":%d,"pointsFound":0,"averageRank":null,"top3Count":0,"top10Count":0}`, tt.size*tt.size))
			if result.MatchedBusiness != nil {
				t.Errorf("matchedBusiness = %+v", result.MatchedBusiness)
			}
		})
	}
}

func TestLocalRankGridMatching(t *testing.T) {
	for _, tt := range []struct {
		name, target, items, rank, matched string
	}{
		{"CID", `{"cid":"123"}`, `[{"cid":"other","rank_absolute":1},{"cid":"123","rank_absolute":2,"rank_group":1}]`, "2", `{"title":null,"cid":"123","placeId":null}`},
		{"place ID fallback", `{"cid":"missing","placeId":"p1"}`, `[{"cid":"other","place_id":"p1","rank_group":4}]`, "4", `{"title":null,"cid":"other","placeId":"p1"}`},
		{"name fallback", `{"cid":"missing","placeId":"missing","name":"acme cafe"}`, `[{"title":"ACME Cafe Downtown","rank_absolute":12}]`, "12", `{"title":"ACME Cafe Downtown","cid":null,"placeId":null}`},
		{"first matching row", `{"cid":"123","name":"acme"}`, `[{"title":"Acme Annex","cid":"456","rank_absolute":1},{"title":"Acme Main","cid":"123","rank_absolute":2}]`, "1", `{"title":"Acme Annex","cid":"456","placeId":null}`},
		{"null absolute fallback", `{"cid":"123"}`, `[{"cid":"123","rank_absolute":null,"rank_group":5}]`, "5", `{"title":null,"cid":"123","placeId":null}`},
		{"zero absolute retained", `{"cid":"123"}`, `[{"cid":"123","rank_absolute":0,"rank_group":5}]`, "0", `{"title":null,"cid":"123","placeId":null}`},
		{"wrong absolute type no fallback", `{"cid":"123"}`, `[{"cid":"123","rank_absolute":"2","rank_group":5}]`, "null", `{"title":null,"cid":"123","placeId":null}`},
		{"missing rank", `{"cid":"123"}`, `[{"cid":"123","title":false,"place_id":7}]`, "null", `{"title":null,"cid":"123","placeId":null}`},
		{"numeric CID does not match", `{"cid":"123"}`, `[{"cid":123,"rank_absolute":1}]`, "null", "null"},
		{"CID case sensitive", `{"cid":"abc"}`, `[{"cid":"ABC","rank_absolute":1}]`, "null", "null"},
		{"no match", `{"name":"acme"}`, `[null,{"title":5,"rank_absolute":1},{"title":"Other","rank_absolute":2}]`, "null", "null"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var target LocalRankGridTarget
			if err := json.Unmarshal([]byte(tt.target), &target); err != nil {
				t.Fatal(err)
			}
			c, _ := newKWClient(t, func(string, map[string]any) kwReply {
				return kwReply{Result: json.RawMessage(`[{"items":` + tt.items + `}]`)}
			})
			result, err := c.LocalRankGrid(context.Background(), LocalRankGridRequest{
				Keyword: "coffee", Target: &target,
				Center: &LocalRankGridCenter{Latitude: domainTestPtr(40.0), Longitude: domainTestPtr(-74.0)},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, point := range result.Grid {
				domainTestJSON(t, point.Rank, tt.rank)
			}
			domainTestJSON(t, result.MatchedBusiness, tt.matched)
		})
	}
}

func TestLocalRankGridAverageRounding(t *testing.T) {
	c, _ := newKWClient(t, func(_ string, task map[string]any) kwReply {
		items := `[]`
		switch task["location_coordinate"] {
		case "40.0180874,-74.0234532,13z":
			items = `[{"cid":"123","rank_absolute":1}]`
		case "40.0180874,-74,13z", "40.0180874,-73.9765468,13z":
			items = `[{"cid":"123","rank_absolute":2}]`
		}
		return kwReply{Result: json.RawMessage(`[{"items":` + items + `}]`)}
	})
	result, err := c.LocalRankGrid(context.Background(), LocalRankGridRequest{
		Keyword: "coffee", Target: &LocalRankGridTarget{CID: domainTestPtr("123")},
		Center: &LocalRankGridCenter{Latitude: domainTestPtr(40.0), Longitude: domainTestPtr(-74.0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	domainTestJSON(t, result.Summary, `{"pointsSearched":9,"pointsFound":3,"averageRank":1.67,"top3Count":3,"top10Count":3}`)
}

func TestLocalRankGridInputErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*LocalRankGridRequest)
	}{
		{"keyword empty", func(r *LocalRankGridRequest) { r.Keyword = "" }},
		{"keyword too long", func(r *LocalRankGridRequest) { r.Keyword = strings.Repeat("a", 121) }},
		{"target missing", func(r *LocalRankGridRequest) { r.Target = nil }},
		{"target empty", func(r *LocalRankGridRequest) { r.Target = &LocalRankGridTarget{} }},
		{"CID empty", func(r *LocalRankGridRequest) { r.Target.CID = domainTestPtr("") }},
		{"CID too long", func(r *LocalRankGridRequest) { r.Target.CID = domainTestPtr(strings.Repeat("a", 65)) }},
		{"place ID empty", func(r *LocalRankGridRequest) { r.Target.PlaceID = domainTestPtr("") }},
		{"place ID too long", func(r *LocalRankGridRequest) { r.Target.PlaceID = domainTestPtr(strings.Repeat("a", 257)) }},
		{"name empty", func(r *LocalRankGridRequest) { r.Target.Name = domainTestPtr("") }},
		{"name too long", func(r *LocalRankGridRequest) { r.Target.Name = domainTestPtr(strings.Repeat("a", 201)) }},
		{"center missing", func(r *LocalRankGridRequest) { r.Center = nil }},
		{"latitude missing", func(r *LocalRankGridRequest) { r.Center.Latitude = nil }},
		{"longitude missing", func(r *LocalRankGridRequest) { r.Center.Longitude = nil }},
		{"latitude low", func(r *LocalRankGridRequest) { r.Center.Latitude = domainTestPtr(-91.0) }},
		{"latitude high", func(r *LocalRankGridRequest) { r.Center.Latitude = domainTestPtr(91.0) }},
		{"longitude low", func(r *LocalRankGridRequest) { r.Center.Longitude = domainTestPtr(-181.0) }},
		{"longitude high", func(r *LocalRankGridRequest) { r.Center.Longitude = domainTestPtr(181.0) }},
		{"latitude NaN", func(r *LocalRankGridRequest) { r.Center.Latitude = domainTestPtr(math.NaN()) }},
		{"longitude infinity", func(r *LocalRankGridRequest) { r.Center.Longitude = domainTestPtr(math.Inf(1)) }},
		{"grid size zero", func(r *LocalRankGridRequest) { r.GridSize = domainTestPtr(0) }},
		{"grid size four", func(r *LocalRankGridRequest) { r.GridSize = domainTestPtr(4) }},
		{"grid size seven", func(r *LocalRankGridRequest) { r.GridSize = domainTestPtr(7) }},
		{"spacing low", func(r *LocalRankGridRequest) { r.SpacingKm = domainTestPtr(0.24) }},
		{"spacing high", func(r *LocalRankGridRequest) { r.SpacingKm = domainTestPtr(10.1) }},
		{"spacing NaN", func(r *LocalRankGridRequest) { r.SpacingKm = domainTestPtr(math.NaN()) }},
		{"spacing infinity", func(r *LocalRankGridRequest) { r.SpacingKm = domainTestPtr(math.Inf(1)) }},
		{"zoom low", func(r *LocalRankGridRequest) { r.Zoom = domainTestPtr(3) }},
		{"zoom high", func(r *LocalRankGridRequest) { r.Zoom = domainTestPtr(19) }},
		{"device", func(r *LocalRankGridRequest) { r.Device = "tablet" }},
		{"language", func(r *LocalRankGridRequest) { r.LanguageCode = "xx" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := LocalRankGridRequest{Keyword: "coffee", Target: &LocalRankGridTarget{CID: domainTestPtr("123")}, Center: &LocalRankGridCenter{Latitude: domainTestPtr(0.0), Longitude: domainTestPtr(0.0)}}
			tt.change(&req)
			_, err := New(nil).LocalRankGrid(context.Background(), req)
			var inputErr *InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("error = %v, want InputError", err)
			}
		})
	}
}

func TestLocalRankGridFailures(t *testing.T) {
	for _, tt := range []struct {
		name               string
		httpStatus, status int
		message            string
		wantCalls          int
		wantError          bool
	}{
		{"all empty", 200, 40501, "No Search Results.", 9, false},
		{"all ordinary failures", 200, 40101, "Internal SE Server Error.", 9, true},
		{"invalid coordinate field", 200, 40501, "Invalid Field: 'location_coordinate'.", 9, true},
		{"HTTP auth abort", 401, 0, "Unauthorized", 3, true},
		{"task auth follows point handling", 200, 40100, "Unauthorized", 9, true},
		{"task balance follows point handling", 200, 40200, "Payment Required.", 9, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != pathLocalSerpMaps {
					t.Errorf("path = %s", r.URL.Path)
				}
				if tt.httpStatus != 200 {
					http.Error(w, tt.message, tt.httpStatus)
					return
				}
				domainTestResponse(w, tt.status, tt.message, "null")
			})
			result, err := c.LocalRankGrid(context.Background(), LocalRankGridRequest{Keyword: "coffee", Target: &LocalRankGridTarget{CID: domainTestPtr("123")}, Center: &LocalRankGridCenter{Latitude: domainTestPtr(40.0), Longitude: domainTestPtr(-74.0)}})
			if tt.wantError {
				var provider *dataforseo.Error
				if result != nil || !errors.As(err, &provider) || provider.StatusCode != tt.status || (tt.httpStatus != 200 && provider.HTTPStatus != tt.httpStatus) {
					t.Fatalf("result = %+v, error = %v", result, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				domainTestJSON(t, result.Summary, `{"pointsSearched":9,"pointsFound":0,"averageRank":null,"top3Count":0,"top10Count":0}`)
				for _, point := range result.Grid {
					if point.Error {
						t.Error("no results marked as failed")
					}
				}
			}
			if got := int(calls.Load()); (tt.httpStatus == 401 && (got < 1 || got > tt.wantCalls)) || (tt.httpStatus != 401 && got != tt.wantCalls) {
				t.Errorf("calls = %d, want %d (at most for abort)", got, tt.wantCalls)
			}
		})
	}
}

func TestLocalRankGridBoundedConcurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entered := make(chan struct{}, 25)
	release := make(chan struct{})
	var once sync.Once
	releaseBatch := func() { once.Do(func() { close(release) }) }
	defer releaseBatch()
	var active, maximum, calls atomic.Int32
	c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, n) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return
		}
		domainTestResponse(w, 20000, "Ok.", `[{"items":[]}]`)
	})
	done := make(chan error, 1)
	go func() {
		_, err := c.LocalRankGrid(ctx, LocalRankGridRequest{Keyword: "coffee", Target: &LocalRankGridTarget{CID: domainTestPtr("123")}, Center: &LocalRankGridCenter{Latitude: domainTestPtr(40.0), Longitude: domainTestPtr(-74.0)}, GridSize: domainTestPtr(5)})
		done <- err
	}()
	for range 3 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("three searches did not run concurrently")
		}
	}
	select {
	case <-entered:
		t.Fatal("more than three searches in flight")
	case <-time.After(30 * time.Millisecond):
	}
	releaseBatch()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("grid did not finish")
	}
	if maximum.Load() != 3 || calls.Load() != 25 {
		t.Errorf("max concurrent = %d, calls = %d", maximum.Load(), calls.Load())
	}
}

func TestLocalSerpCancellation(t *testing.T) {
	for _, grid := range []bool{false, true} {
		for _, before := range []bool{false, true} {
			t.Run(fmt.Sprintf("grid=%t/before=%t", grid, before), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				var calls atomic.Int32
				started := make(chan struct{}, 3)
				c := domainTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					var tasks []map[string]any
					if err := json.NewDecoder(r.Body).Decode(&tasks); err != nil {
						t.Error(err)
						return
					}
					calls.Add(1)
					started <- struct{}{}
					select {
					case <-r.Context().Done():
					case <-time.After(3 * time.Second):
						t.Error("HTTP request was not canceled")
					}
				})
				if before {
					cancel()
				}
				done := make(chan error, 1)
				go func() {
					var err error
					if grid {
						_, err = c.LocalRankGrid(ctx, LocalRankGridRequest{Keyword: "coffee", Target: &LocalRankGridTarget{CID: domainTestPtr("123")}, Center: &LocalRankGridCenter{Latitude: domainTestPtr(40.0), Longitude: domainTestPtr(-74.0)}})
					} else {
						_, err = c.LocalSerpResults(ctx, LocalSerpResultsRequest{Keyword: "coffee", Near: &LocalSerpNear{Latitude: domainTestPtr(40.0), Longitude: domainTestPtr(-74.0)}})
					}
					done <- err
				}()
				if !before {
					select {
					case <-started:
					case <-time.After(3 * time.Second):
						t.Fatal("request did not start")
					}
					cancel()
				}
				select {
				case err := <-done:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("error = %v", err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("operation did not stop")
				}
				got := calls.Load()
				if (before && got != 0) || (!before && ((!grid && got != 1) || (grid && (got < 1 || got > 3)))) {
					t.Errorf("calls = %d", got)
				}
			})
		}
	}
}
