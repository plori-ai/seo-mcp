package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
)

func TestLocalSerpToolSchemas(t *testing.T) {
	for _, tt := range []struct {
		tool       Tool
		required   []string
		coordinate string
	}{
		{localSerpResultsTool, []string{"keyword", "near"}, "near"},
		{localRankGridTool, []string{"keyword", "target", "center"}, "center"},
	} {
		t.Run(tt.tool.Name, func(t *testing.T) {
			var schema struct {
				Type       string                    `json:"type"`
				Properties map[string]map[string]any `json:"properties"`
				Required   []string                  `json:"required"`
			}
			if err := json.Unmarshal(tt.tool.InputSchema, &schema); err != nil {
				t.Fatal(err)
			}
			if schema.Type != "object" || !reflect.DeepEqual(schema.Required, tt.required) {
				t.Fatalf("schema = %s", tt.tool.InputSchema)
			}
			for _, text := range []string{tt.tool.Description, string(tt.tool.InputSchema)} {
				for _, forbidden := range []string{"project", "credits", "cached", "openseo app"} {
					if strings.Contains(strings.ToLower(text), forbidden) {
						t.Errorf("contains %q", forbidden)
					}
				}
			}
			if !strings.Contains(tt.tool.Description, "billed by DataForSEO to the operator's account") {
				t.Error("missing billing description")
			}
			props := schema.Properties
			if props["keyword"]["minLength"] != float64(1) || props["keyword"]["maxLength"] != float64(120) {
				t.Error("keyword bounds missing")
			}
			if !reflect.DeepEqual(props["device"]["enum"], []any{"desktop", "mobile"}) {
				t.Error("device enum missing")
			}
			coordinate := props[tt.coordinate]
			if !reflect.DeepEqual(coordinate["required"], []any{"latitude", "longitude"}) {
				t.Error("required coordinates missing")
			}
			coordinateProps := coordinate["properties"].(map[string]any)
			for _, field := range []struct {
				name  string
				bound float64
			}{{"latitude", 90}, {"longitude", 180}} {
				property := coordinateProps[field.name].(map[string]any)
				if property["minimum"] != -field.bound || property["maximum"] != field.bound || property["type"] != "number" {
					t.Errorf("%s bounds = %v", field.name, property)
				}
			}
			var zoom map[string]any
			if tt.tool.Name == "get_local_serp_results" {
				if props["depth"]["minimum"] != float64(1) || props["depth"]["maximum"] != float64(100) || props["depth"]["type"] != "integer" {
					t.Error("depth bounds missing")
				}
				if !reflect.DeepEqual(props["searchType"]["enum"], []any{"maps", "local_finder"}) {
					t.Error("searchType enum missing")
				}
				zoom = coordinateProps["zoom"].(map[string]any)
			} else {
				if !reflect.DeepEqual(props["gridSize"]["enum"], []any{float64(3), float64(5)}) {
					t.Error("grid sizes must include 3 and 5")
				}
				if props["spacingKm"]["minimum"] != 0.25 || props["spacingKm"]["maximum"] != float64(10) {
					t.Error("spacing bounds missing")
				}
				if !strings.Contains(tt.tool.Description, "gridSize squared") || !strings.Contains(tt.tool.Description, "9 requests") || !strings.Contains(tt.tool.Description, "25") {
					t.Error("grid request count missing")
				}
				wantAny := []any{map[string]any{"required": []any{"cid"}}, map[string]any{"required": []any{"placeId"}}, map[string]any{"required": []any{"name"}}}
				if !reflect.DeepEqual(props["target"]["anyOf"], wantAny) {
					t.Error("target must require at least one identifier")
				}
				zoom = props["zoom"]
			}
			if zoom["minimum"] != float64(4) || zoom["maximum"] != float64(18) || zoom["type"] != "integer" {
				t.Error("zoom bounds missing")
			}
		})
	}
}

func TestLocalSerpToolsCall(t *testing.T) {
	for _, tt := range []struct {
		name, tool, args, path, coordinate, language, device, os string
		depth, count                                             int
	}{
		{"snapshot defaults", "get_local_serp_results", `{"keyword":"coffee","near":{"latitude":40,"longitude":-74}}`, "maps", "40,-74", "de", "mobile", "android", 20, 1},
		{"finder explicit", "get_local_serp_results", `{"keyword":"coffee","near":{"latitude":40,"longitude":-74,"zoom":18},"searchType":"local_finder","device":"desktop","depth":100,"languageCode":"fr"}`, "local_finder", "40,-74,18z", "fr", "desktop", "windows", 100, 1},
		{"grid defaults", "get_local_rank_grid", `{"keyword":"coffee","target":{"cid":"123"},"center":{"latitude":40,"longitude":-74}}`, "maps", "", "de", "mobile", "android", 20, 9},
		{"grid explicit", "get_local_rank_grid", `{"keyword":"coffee","target":{"cid":"123","placeId":"p1","name":"Acme"},"center":{"latitude":40,"longitude":-74},"gridSize":5,"spacingKm":0.25,"zoom":18,"device":"desktop","languageCode":"fr"}`, "maps", "", "fr", "desktop", "windows", 20, 25},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/v3/serp/google/"+tt.path+"/live/advanced" || r.Method != http.MethodPost {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				var tasks []map[string]any
				if err := json.NewDecoder(r.Body).Decode(&tasks); err != nil || len(tasks) != 1 {
					t.Errorf("tasks = %v, error = %v", tasks, err)
					return
				}
				coordinate := tt.coordinate
				if coordinate == "" {
					coordinate, _ = tasks[0]["location_coordinate"].(string)
					if coordinate == "" {
						t.Error("grid coordinate missing")
					}
				}
				want := map[string]any{"keyword": "coffee", "location_coordinate": coordinate, "language_code": tt.language, "device": tt.device, "os": tt.os, "depth": float64(tt.depth)}
				if tt.path == "maps" {
					want["search_places"] = false
				}
				if !reflect.DeepEqual(tasks[0], want) {
					t.Errorf("task = %v, want %v", tasks[0], want)
				}
				_, _ = w.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"result":[{"items":[{"title":"Acme","cid":"123","rank_absolute":2,"main_image":"omitted"}]}]}]}`))
			}))
			defer server.Close()
			api := dataforseo.New("synthetic-test-key")
			api.BaseURL = server.URL
			result, err := New(seo.New(api, seo.WithDefaultMarket(seo.Market{LocationCode: 2276, LanguageCode: "de"}))).Call(context.Background(), tt.tool, json.RawMessage(tt.args))
			if err != nil {
				t.Fatal(err)
			}
			if int(calls.Load()) != tt.count {
				t.Errorf("calls = %d, want %d", calls.Load(), tt.count)
			}
			switch result := result.(type) {
			case *seo.LocalSerpResultsResult:
				raw, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				var got any
				if err := json.Unmarshal(raw, &got); err != nil {
					t.Fatal(err)
				}
				want := map[string]any{"results": []any{map[string]any{"title": "Acme", "cid": "123", "rank_absolute": float64(2)}}}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("result = %s", raw)
				}
			case *seo.LocalRankGridResult:
				if len(result.Grid) != tt.count || result.Summary.PointsFound != tt.count || result.Summary.AverageRank == nil || *result.Summary.AverageRank != 2 || result.MatchedBusiness == nil {
					t.Errorf("result = %+v", result)
				}
			default:
				t.Fatalf("result type %T", result)
			}
		})
	}
}

func TestLocalSerpToolArgumentErrors(t *testing.T) {
	for _, tt := range []struct{ tool, args string }{
		{"get_local_serp_results", `{}`},
		{"get_local_serp_results", `{"keyword":42}`},
		{"get_local_serp_results", `{"keyword":"coffee","near":{}}`},
		{"get_local_serp_results", `{"keyword":"coffee","near":{"latitude":0,"longitude":0,"zoom":4.5}}`},
		{"get_local_serp_results", `{"keyword":"coffee","near":{"latitude":0,"longitude":0},"depth":0}`},
		{"get_local_serp_results", `{"keyword":"coffee","near":{"latitude":0,"longitude":0},"depth":1.5}`},
		{"get_local_rank_grid", `{}`},
		{"get_local_rank_grid", `{"keyword":"coffee","target":{},"center":{"latitude":0,"longitude":0}}`},
		{"get_local_rank_grid", `{"keyword":"coffee","target":{"cid":"123"},"center":{"latitude":0}}`},
		{"get_local_rank_grid", `{"keyword":"coffee","target":{"cid":123},"center":{"latitude":0,"longitude":0}}`},
		{"get_local_rank_grid", `{"keyword":"coffee","target":{"cid":"123"},"center":{"latitude":0,"longitude":0},"gridSize":4}`},
		{"get_local_rank_grid", `{"keyword":"coffee","target":{"cid":"123"},"center":{"latitude":0,"longitude":0},"spacingKm":0}`},
	} {
		t.Run(tt.tool+"/"+tt.args, func(t *testing.T) {
			_, err := New(seo.New(nil)).Call(context.Background(), tt.tool, json.RawMessage(tt.args))
			var inputErr *seo.InputError
			if !errors.As(err, &inputErr) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
