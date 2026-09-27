package seo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

const (
	pathLocalSerpMaps   = "/v3/serp/google/maps/live/advanced"
	pathLocalSerpFinder = "/v3/serp/google/local_finder/live/advanced"
	rankGridDepth       = 20
	rankGridConcurrency = 3
)

// LocalSerpResultsRequest selects one Google Maps or Local Finder snapshot.
// Nil Depth means 20; empty SearchType and Device mean maps and mobile.
type LocalSerpResultsRequest struct {
	Keyword      string         `json:"keyword"`
	Near         *LocalSerpNear `json:"near"`
	SearchType   string         `json:"searchType,omitempty"`
	Device       string         `json:"device,omitempty"`
	Depth        *int           `json:"depth,omitempty"`
	LanguageCode string         `json:"languageCode,omitempty"`
}

// LocalSerpNear specifies the search coordinate and optional map zoom.
// Latitude and Longitude are required pointers so omitted coordinates cannot
// silently become zero. Zero itself is a valid coordinate.
type LocalSerpNear struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Zoom      *int     `json:"zoom,omitempty"`
}

// LocalSerpResultsResult contains the reference tool's selected provider
// fields, retaining nested objects, explicit nulls, and absent fields.
type LocalSerpResultsResult struct {
	Results []map[string]json.RawMessage `json:"results"`
}

// LocalRankGridRequest selects a square Maps grid, checking the first 20
// results at each point. Nil GridSize and SpacingKm mean 3 and 2 km. Nil Zoom
// derives a shared zoom from the spacing and center latitude.
type LocalRankGridRequest struct {
	Keyword      string               `json:"keyword"`
	Target       *LocalRankGridTarget `json:"target"`
	Center       *LocalRankGridCenter `json:"center"`
	GridSize     *int                 `json:"gridSize,omitempty"`
	SpacingKm    *float64             `json:"spacingKm,omitempty"`
	Device       string               `json:"device,omitempty"`
	Zoom         *int                 `json:"zoom,omitempty"`
	LanguageCode string               `json:"languageCode,omitempty"`
}

// LocalRankGridTarget requires at least one identifier. A row matches if its
// CID or PlaceID equals the supplied identifier, or its title contains Name
// without regard to case. The first matching row supplies the rank.
type LocalRankGridTarget struct {
	CID     *string `json:"cid,omitempty"`
	PlaceID *string `json:"placeId,omitempty"`
	Name    *string `json:"name,omitempty"`
}

// LocalRankGridCenter is the storefront or other center of a rank grid.
// Both coordinates are required; pointers distinguish omission from zero.
type LocalRankGridCenter struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// LocalRankGridResult lists points in row-major order, north to south.
type LocalRankGridResult struct {
	Grid            []LocalRankGridPoint          `json:"grid"`
	Summary         LocalRankGridSummary          `json:"summary"`
	MatchedBusiness *LocalRankGridMatchedBusiness `json:"matchedBusiness"`
}

// LocalRankGridPoint reports a rank or null when the target was not ranked.
// Failed points carry Error and omit ResultsCount and TopResult in JSON.
type LocalRankGridPoint struct {
	Row          int                     `json:"row"`
	Col          int                     `json:"col"`
	Latitude     float64                 `json:"latitude"`
	Longitude    float64                 `json:"longitude"`
	Rank         *float64                `json:"rank"`
	ResultsCount *int                    `json:"resultsCount,omitempty"`
	TopResult    *LocalRankGridTopResult `json:"topResult"`
	Error        bool                    `json:"error,omitempty"`
}

// MarshalJSON distinguishes failed searches from successful empty SERPs.
func (p LocalRankGridPoint) MarshalJSON() ([]byte, error) {
	if p.Error {
		return json.Marshal(struct {
			Row       int      `json:"row"`
			Col       int      `json:"col"`
			Latitude  float64  `json:"latitude"`
			Longitude float64  `json:"longitude"`
			Rank      *float64 `json:"rank"`
			Error     bool     `json:"error"`
		}{p.Row, p.Col, p.Latitude, p.Longitude, nil, true})
	}
	type plain LocalRankGridPoint
	return json.Marshal(plain(p))
}

// LocalRankGridTopResult identifies the first returned business at a point.
type LocalRankGridTopResult struct {
	Title *string `json:"title"`
	CID   *string `json:"cid"`
}

// LocalRankGridMatchedBusiness identifies the first match in completion order.
type LocalRankGridMatchedBusiness struct {
	Title   *string `json:"title"`
	CID     *string `json:"cid"`
	PlaceID *string `json:"placeId"`
}

// LocalRankGridSummary counts all searched points, including failures, and
// averages only numeric ranks. AverageRank is null when no rank was found.
type LocalRankGridSummary struct {
	PointsSearched int      `json:"pointsSearched"`
	PointsFound    int      `json:"pointsFound"`
	AverageRank    *float64 `json:"averageRank"`
	Top3Count      int      `json:"top3Count"`
	Top10Count     int      `json:"top10Count"`
}

var localSerpRowFields = []string{
	"rank_group", "rank_absolute", "title", "domain", "url", "contact_url",
	"address", "address_info", "phone", "category", "additional_categories",
	"rating", "rating_distribution", "price_level", "is_claimed", "cid",
	"place_id", "latitude", "longitude", "total_photos", "work_hours",
	"local_justifications",
}

// LocalSerpResults returns local business rankings near a coordinate in one
// DataForSEO request. A provider "No Search Results" response is an empty list.
func (c *Client) LocalSerpResults(ctx context.Context, req LocalSerpResultsRequest) (*LocalSerpResultsResult, error) {
	if err := validateLocalSerpKeyword(req.Keyword); err != nil {
		return nil, err
	}
	if req.Near == nil {
		return nil, inputErrorf("near is required")
	}
	if err := validateLocalSerpCoordinate("near", req.Near.Latitude, req.Near.Longitude); err != nil {
		return nil, err
	}
	if err := validateLocalSerpZoom(req.Near.Zoom); err != nil {
		return nil, err
	}
	searchType := req.SearchType
	if searchType == "" {
		searchType = "maps"
	}
	if searchType != "maps" && searchType != "local_finder" {
		return nil, inputErrorf("searchType must be maps or local_finder")
	}
	device, err := localSerpDevice(req.Device)
	if err != nil {
		return nil, err
	}
	depth := 20
	if req.Depth != nil {
		depth = *req.Depth
	}
	if depth < 1 || depth > 100 {
		return nil, inputErrorf("depth must be between 1 and 100")
	}
	language, err := c.localSerpLanguage(req.LanguageCode)
	if err != nil {
		return nil, err
	}
	coordinate := formatLocalSerpCoordinate(*req.Near.Latitude, *req.Near.Longitude, req.Near.Zoom)
	rows, err := c.fetchLocalSerp(ctx, req.Keyword, coordinate, language, searchType, device, depth)
	if err != nil {
		return nil, err
	}
	results := make([]map[string]json.RawMessage, 0, len(rows))
	for _, row := range rows {
		selected := make(map[string]json.RawMessage)
		for _, field := range localSerpRowFields {
			if value, ok := row[field]; ok {
				selected[field] = value
			}
		}
		results = append(results, selected)
	}
	return &LocalSerpResultsResult{Results: results}, nil
}

func (c *Client) fetchLocalSerp(ctx context.Context, keyword, coordinate, language, searchType, device string, depth int) ([]map[string]json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	os := "android"
	if device == "desktop" {
		os = "windows"
	}
	payload := map[string]any{
		"keyword": keyword, "location_coordinate": coordinate,
		"language_code": language, "device": device, "os": os, "depth": depth,
	}
	path := pathLocalSerpFinder
	if searchType == "maps" {
		path = pathLocalSerpMaps
		payload["search_places"] = false
	}
	task, err := c.api.Post(ctx, path, payload)
	if dataforseo.IsNoResults(err) {
		return []map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("local SERP: %w", err)
	}
	var result struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if _, err := task.FirstResult(&result); err != nil {
		return nil, err
	}
	return result.Items, nil
}

// LocalRankGrid makes GridSize squared Maps requests, with at most three in
// flight. Ordinary failures affect only their points; an entirely failed grid
// returns the last error. HTTP 401 stops later batches. Context
// cancellation stops pending work and returns the context error.
func (c *Client) LocalRankGrid(ctx context.Context, req LocalRankGridRequest) (*LocalRankGridResult, error) {
	if err := validateLocalRankGrid(req); err != nil {
		return nil, err
	}
	device, err := localSerpDevice(req.Device)
	if err != nil {
		return nil, err
	}
	language, err := c.localSerpLanguage(req.LanguageCode)
	if err != nil {
		return nil, err
	}
	gridSize, spacing := 3, 2.0
	if req.GridSize != nil {
		gridSize = *req.GridSize
	}
	if req.SpacingKm != nil {
		spacing = *req.SpacingKm
	}
	zoom := localRankGridZoom(spacing, *req.Center.Latitude)
	if req.Zoom != nil {
		zoom = *req.Zoom
	}
	result := &LocalRankGridResult{Grid: buildLocalRankGridPoints(*req.Center.Latitude, *req.Center.Longitude, gridSize, spacing)}
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	type pointReply struct {
		index int
		items []map[string]json.RawMessage
		err   error
	}
	var lastError, abortError error
	failures := 0
	for start := 0; start < len(result.Grid); start += rankGridConcurrency {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		end := min(start+rankGridConcurrency, len(result.Grid))
		replies := make(chan pointReply, end-start)
		for index := start; index < end; index++ {
			point := result.Grid[index]
			go func() {
				coordinate := formatLocalSerpCoordinate(point.Latitude, point.Longitude, &zoom)
				items, err := c.fetchLocalSerp(workCtx, req.Keyword, coordinate, language, "maps", device, rankGridDepth)
				replies <- pointReply{index, items, err}
			}()
		}
		// Consume completion order for matchedBusiness and lastError, while
		// storing points in their original geographic order.
		for remaining := end - start; remaining > 0; remaining-- {
			reply := <-replies
			point := &result.Grid[reply.index]
			if reply.err != nil {
				point.Error = true
				failures++
				lastError = reply.err
				if abortError == nil && abortLocalRankGrid(reply.err) {
					abortError = reply.err
					cancel()
				}
				continue
			}
			count := len(reply.items)
			point.ResultsCount = &count
			if count > 0 && reply.items[0] != nil {
				point.TopResult = &LocalRankGridTopResult{
					Title: localSerpString(reply.items[0]["title"]), CID: localSerpString(reply.items[0]["cid"]),
				}
			}
			match := matchLocalRankGridItem(reply.items, *req.Target)
			if match != nil {
				if result.MatchedBusiness == nil {
					result.MatchedBusiness = &LocalRankGridMatchedBusiness{
						Title: localSerpString(match["title"]), CID: localSerpString(match["cid"]), PlaceID: localSerpString(match["place_id"]),
					}
				}
				rank := match["rank_absolute"]
				if len(rank) == 0 || string(rank) == "null" {
					rank = match["rank_group"]
				}
				var number *float64
				if json.Unmarshal(rank, &number) == nil {
					point.Rank = number
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if abortError != nil {
			return nil, abortError
		}
	}
	if failures == len(result.Grid) {
		return nil, lastError
	}
	result.Summary.PointsSearched = len(result.Grid)
	var total float64
	for _, point := range result.Grid {
		if point.Rank == nil {
			continue
		}
		rank := *point.Rank
		total += rank
		result.Summary.PointsFound++
		if rank <= 3 {
			result.Summary.Top3Count++
		}
		if rank <= 10 {
			result.Summary.Top10Count++
		}
	}
	if result.Summary.PointsFound > 0 {
		average := localSerpRound(total/float64(result.Summary.PointsFound), 2)
		result.Summary.AverageRank = &average
	}
	return result, nil
}

func validateLocalRankGrid(req LocalRankGridRequest) error {
	if err := validateLocalSerpKeyword(req.Keyword); err != nil {
		return err
	}
	if req.Target == nil || (req.Target.CID == nil && req.Target.PlaceID == nil && req.Target.Name == nil) {
		return inputErrorf("target needs at least one of cid, placeId, or name")
	}
	for _, field := range []struct {
		name  string
		value *string
		limit int
	}{{"cid", req.Target.CID, 64}, {"placeId", req.Target.PlaceID, 256}, {"name", req.Target.Name, 200}} {
		if field.value != nil {
			if n := researchStringLength(*field.value); n < 1 || n > field.limit {
				return inputErrorf("target.%s must contain 1 to %d characters", field.name, field.limit)
			}
		}
	}
	if req.Center == nil {
		return inputErrorf("center is required")
	}
	if err := validateLocalSerpCoordinate("center", req.Center.Latitude, req.Center.Longitude); err != nil {
		return err
	}
	if req.GridSize != nil && *req.GridSize != 3 && *req.GridSize != 5 {
		return inputErrorf("gridSize must be 3 or 5")
	}
	if req.SpacingKm != nil && (math.IsNaN(*req.SpacingKm) || *req.SpacingKm < 0.25 || *req.SpacingKm > 10) {
		return inputErrorf("spacingKm must be between 0.25 and 10")
	}
	return validateLocalSerpZoom(req.Zoom)
}

func validateLocalSerpKeyword(keyword string) error {
	if n := researchStringLength(keyword); n < 1 || n > 120 {
		return inputErrorf("keyword must contain 1 to 120 characters")
	}
	return nil
}

func validateLocalSerpCoordinate(field string, latitude, longitude *float64) error {
	if latitude == nil || math.IsNaN(*latitude) || *latitude < -90 || *latitude > 90 {
		return inputErrorf("%s.latitude is required and must be between -90 and 90", field)
	}
	if longitude == nil || math.IsNaN(*longitude) || *longitude < -180 || *longitude > 180 {
		return inputErrorf("%s.longitude is required and must be between -180 and 180", field)
	}
	return nil
}

func validateLocalSerpZoom(zoom *int) error {
	if zoom != nil && (*zoom < 4 || *zoom > 18) {
		return inputErrorf("zoom must be between 4 and 18")
	}
	return nil
}

func localSerpDevice(device string) (string, error) {
	if device == "" {
		return "mobile", nil
	}
	if device != "mobile" && device != "desktop" {
		return "", inputErrorf("device must be desktop or mobile")
	}
	return device, nil
}

func (c *Client) localSerpLanguage(language string) (string, error) {
	if language == "" {
		market, err := c.resolveMarket(0, "")
		return market.LanguageCode, err
	}
	// Coordinate searches accept SERP languages independently of the default
	// country's Labs language list.
	if !supportedLanguages[language] {
		return "", inputErrorf("language code %q is not supported. Use a DataForSEO language code such as 'en', 'es', 'de' or 'fr'.", language)
	}
	return language, nil
}

func formatLocalSerpCoordinate(latitude, longitude float64, zoom *int) string {
	format := func(value float64) string {
		value = localSerpRound(value, 7)
		if value == 0 {
			return "0"
		}
		// JavaScript prints magnitudes below 1e-6 in exponent notation.
		if math.Abs(value) < 1e-6 {
			return strings.Replace(strconv.FormatFloat(value, 'e', -1, 64), "e-0", "e-", 1)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	coordinate := format(latitude) + "," + format(longitude)
	if zoom != nil {
		coordinate += "," + strconv.Itoa(*zoom) + "z"
	}
	return coordinate
}

// JavaScript toFixed rounds the exact binary value, with ties away from zero.
// A rational avoids both ties-to-even and an extra floating-point multiply.
func localSerpRound(value float64, places int) float64 {
	scale := int64(math.Pow10(places))
	ratio := new(big.Rat).SetFloat64(math.Abs(value))
	ratio.Mul(ratio, new(big.Rat).SetInt64(scale))
	whole, remainder := new(big.Int), new(big.Int)
	whole.QuoRem(ratio.Num(), ratio.Denom(), remainder)
	if remainder.Lsh(remainder, 1).Cmp(ratio.Denom()) >= 0 {
		whole.Add(whole, big.NewInt(1))
	}
	rounded, _ := new(big.Rat).SetFrac(whole, big.NewInt(scale)).Float64()
	return math.Copysign(rounded, value)
}

func localRankGridZoom(spacing, latitude float64) int {
	cosine := math.Max(math.Abs(math.Cos(latitude*math.Pi/180)), 0.01)
	return min(18, max(4, int(math.Floor(math.Log2(24045*cosine/spacing)))))
}

func buildLocalRankGridPoints(latitude, longitude float64, size int, spacing float64) []LocalRankGridPoint {
	middle := float64(size-1) / 2
	latitudeStep := spacing / 110.574
	longitudeStep := spacing / (111.32 * math.Max(math.Abs(math.Cos(latitude*math.Pi/180)), 0.01))
	points := make([]LocalRankGridPoint, 0, size*size)
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			points = append(points, LocalRankGridPoint{
				Row: row, Col: col,
				Latitude:  localSerpRound(latitude+(middle-float64(row))*latitudeStep, 7),
				Longitude: localSerpRound(longitude+(float64(col)-middle)*longitudeStep, 7),
			})
		}
	}
	return points
}

func matchLocalRankGridItem(items []map[string]json.RawMessage, target LocalRankGridTarget) map[string]json.RawMessage {
	for _, item := range items {
		if cid := localSerpString(item["cid"]); target.CID != nil && cid != nil && *cid == *target.CID {
			return item
		}
		if placeID := localSerpString(item["place_id"]); target.PlaceID != nil && placeID != nil && *placeID == *target.PlaceID {
			return item
		}
		if title := localSerpString(item["title"]); target.Name != nil && title != nil && strings.Contains(strings.ToLower(*title), strings.ToLower(*target.Name)) {
			return item
		}
	}
	return nil
}

func localSerpString(raw json.RawMessage) *string {
	var value *string
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return value
}

func abortLocalRankGrid(err error) bool {
	var provider *dataforseo.Error
	// The reference local fetcher classifies HTTP 401 as an abort, but leaves
	// task-level auth and payment statuses in ordinary per-point handling.
	return errors.As(err, &provider) && provider.HTTPStatus == http.StatusUnauthorized
}
