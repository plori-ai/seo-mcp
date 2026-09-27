package seo

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"slices"
	"strconv"
	"strings"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

const (
	pathBusinessSearch     = "/v3/business_data/business_listings/search/live"
	pathBusinessCategories = "/v3/business_data/business_listings/categories"
	pathBusinessProfile    = "/v3/business_data/google/my_business_info/live"
	pathBusinessQuestions  = "/v3/business_data/google/questions_and_answers/live"
)

// BusinessSearchNear is the search center and radius for listings and Q&A.
// Coordinate pointers distinguish an omitted coordinate from zero.
type BusinessSearchNear struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	RadiusKM  float64  `json:"radiusKm"`
}

// BusinessProfileNear selects a profile search center. A nil RadiusKM means
// 10 km; an explicit radius must be between 0.2 and 199 km.
type BusinessProfileNear struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	RadiusKM  *float64 `json:"radiusKm,omitempty"`
}

// SearchLocalBusinessesRequest selects listing filters and a result page.
// Nil Limit means 20. Search radii are 1 to 100000 km, rounded to whole km.
type SearchLocalBusinessesRequest struct {
	Query      *string             `json:"query,omitempty"`
	Near       *BusinessSearchNear `json:"near"`
	Categories []string            `json:"categories,omitempty"`
	MinRating  *float64            `json:"minRating,omitempty"`
	MinReviews *int                `json:"minReviews,omitempty"`
	IsClaimed  *bool               `json:"isClaimed,omitempty"`
	SortBy     string              `json:"sortBy,omitempty"`
	Limit      *int                `json:"limit,omitempty"`
	Offset     *int                `json:"offset,omitempty"`
}

// SearchLocalBusinessesResult contains compact listing rows. Missing provider
// fields stay absent; explicit nulls and nested rating fields are preserved.
type SearchLocalBusinessesResult struct {
	Businesses []map[string]json.RawMessage `json:"businesses"`
}

// ListBusinessCategoriesRequest filters category slugs by a case-insensitive
// substring. Nil Limit means 50; the maximum is 200.
type ListBusinessCategoriesRequest struct {
	Query *string `json:"query,omitempty"`
	Limit *int    `json:"limit,omitempty"`
}

// BusinessCategory is a category slug and the number of businesses using it.
type BusinessCategory struct {
	Category      string   `json:"category"`
	BusinessCount *float64 `json:"businessCount"`
}

// ListBusinessCategoriesResult contains categories ranked by business count.
type ListBusinessCategoriesResult struct {
	Categories []BusinessCategory `json:"categories"`
}

// BusinessProfileRequest identifies exactly one business by name, CID or place
// ID. Near takes precedence over LocationCode; zero market fields use defaults.
type BusinessProfileRequest struct {
	BusinessName *string              `json:"businessName,omitempty"`
	CID          *string              `json:"cid,omitempty"`
	PlaceID      *string              `json:"placeId,omitempty"`
	Near         *BusinessProfileNear `json:"near,omitempty"`
	LocationCode int                  `json:"locationCode,omitempty"`
	LanguageCode string               `json:"languageCode,omitempty"`
}

// BusinessProfileResult contains the full provider profile, or null when no
// business matched. Check_url falls back to the result entry's check_url.
type BusinessProfileResult struct {
	Profile map[string]json.RawMessage `json:"profile"`
}

// BusinessQuestionsRequest identifies one business and its search center.
// Nil Depth means 20; the maximum is 100. RadiusKM accepts 1 to 100000 km,
// but the provider coordinate radius is clamped to 199999 meters.
type BusinessQuestionsRequest struct {
	BusinessName *string             `json:"businessName,omitempty"`
	CID          *string             `json:"cid,omitempty"`
	PlaceID      *string             `json:"placeId,omitempty"`
	Near         *BusinessSearchNear `json:"near"`
	Depth        *int                `json:"depth,omitempty"`
	LanguageCode string              `json:"languageCode,omitempty"`
}

// BusinessQuestionsResult combines answered and unanswered questions from all
// result entries. Each row includes compact answers in items, or items: null.
type BusinessQuestionsResult struct {
	Questions []map[string]json.RawMessage `json:"questions"`
}

var businessListingFields = []string{
	"title", "description", "category", "additional_categories", "address",
	"phone", "url", "domain", "rating", "is_claimed", "cid", "place_id",
	"latitude", "longitude", "total_photos", "check_url",
}

var businessQuestionFields = []string{
	"rank_absolute", "question_id", "question_text", "original_question_text",
	"profile_name", "time_ago", "timestamp",
}

var businessAnswerFields = []string{
	"answer_id", "answer_text", "original_answer_text", "profile_name", "time_ago", "timestamp",
}

// SearchLocalBusinesses returns nearby business listings with identity,
// contact, rating and claim information. Each call is billed by DataForSEO.
func (c *Client) SearchLocalBusinesses(ctx context.Context, req SearchLocalBusinessesRequest) (*SearchLocalBusinessesResult, error) {
	if err := validateBusinessSearchNear(req.Near); err != nil {
		return nil, err
	}
	if err := validateBusinessString("query", req.Query, 200); err != nil {
		return nil, err
	}
	if req.Categories != nil {
		if len(req.Categories) < 1 || len(req.Categories) > 10 {
			return nil, inputErrorf("categories must contain 1 to 10 categories")
		}
		for _, category := range req.Categories {
			if err := validateBusinessString("category", &category, 120); err != nil {
				return nil, err
			}
		}
	}
	if req.MinRating != nil && !businessNumberInRange(*req.MinRating, 1, 5) {
		return nil, inputErrorf("minRating must be between 1 and 5")
	}
	if req.MinReviews != nil && *req.MinReviews < 0 {
		return nil, inputErrorf("minReviews must be nonnegative")
	}
	if req.Offset != nil && (*req.Offset < 0 || *req.Offset > 1000) {
		return nil, inputErrorf("offset must be between 0 and 1000")
	}
	limit, err := businessLimit("limit", req.Limit, 20, 50)
	if err != nil {
		return nil, err
	}
	task := map[string]any{
		"location_coordinate": businessCoordinate(*req.Near.Latitude, *req.Near.Longitude, math.Round(req.Near.RadiusKM)),
		"limit":               limit,
	}
	if req.Query != nil {
		task["title"] = *req.Query
	}
	if req.Categories != nil {
		task["categories"] = req.Categories
	}
	if req.IsClaimed != nil {
		task["is_claimed"] = *req.IsClaimed
	}
	if req.Offset != nil {
		task["offset"] = *req.Offset
	}
	var clauses []any
	if req.MinRating != nil {
		clauses = append(clauses, []any{"rating.value", ">=", *req.MinRating})
	}
	if req.MinReviews != nil {
		clauses = append(clauses, []any{"rating.votes_count", ">=", *req.MinReviews})
	}
	if len(clauses) > 0 {
		task["filters"] = joinResearchClauses(clauses, "and")
	}
	switch req.SortBy {
	case "", "relevance":
	case "rating":
		task["order_by"] = []string{"rating.value,desc"}
	case "reviews":
		task["order_by"] = []string{"rating.votes_count,desc"}
	default:
		return nil, inputErrorf("sortBy must be relevance, rating or reviews")
	}
	result := &SearchLocalBusinessesResult{Businesses: []map[string]json.RawMessage{}}
	t, err := c.api.Post(ctx, pathBusinessSearch, task)
	if dataforseo.IsNoResults(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	var response struct {
		Items []json.RawMessage `json:"items"`
	}
	if _, err := t.FirstResult(&response); err != nil {
		return nil, err
	}
	for _, raw := range response.Items {
		result.Businesses = append(result.Businesses, pickBusinessFields(businessObject(raw), businessListingFields))
	}
	return result, nil
}

// ListBusinessCategories fetches the full list on every call, filters it in
// memory, and ranks matches by business count. DataForSEO does not charge for it.
func (c *Client) ListBusinessCategories(ctx context.Context, req ListBusinessCategoriesRequest) (*ListBusinessCategoriesResult, error) {
	if err := validateBusinessString("query", req.Query, 80); err != nil {
		return nil, err
	}
	limit, err := businessLimit("limit", req.Limit, 50, 200)
	if err != nil {
		return nil, err
	}
	t, err := c.api.Get(ctx, pathBusinessCategories)
	if err != nil {
		return nil, err
	}
	var entries []json.RawMessage
	if len(t.Result) > 0 {
		if err := json.Unmarshal(t.Result, &entries); err != nil {
			return nil, fmt.Errorf("business categories: decode result: %w", err)
		}
	}
	query := strings.ToLower(valueOrZero(req.Query))
	result := &ListBusinessCategoriesResult{Categories: []BusinessCategory{}}
	for _, entry := range entries {
		var row struct {
			Category      *string  `json:"category_name"`
			BusinessCount *float64 `json:"business_count"`
		}
		if json.Unmarshal(entry, &row) != nil || row.Category == nil {
			continue
		}
		if strings.Contains(strings.ToLower(*row.Category), query) {
			result.Categories = append(result.Categories, BusinessCategory{Category: *row.Category, BusinessCount: row.BusinessCount})
		}
	}
	slices.SortStableFunc(result.Categories, func(a, b BusinessCategory) int {
		return cmp.Compare(valueOrZero(b.BusinessCount), valueOrZero(a.BusinessCount))
	})
	if len(result.Categories) > limit {
		result.Categories = result.Categories[:limit]
	}
	return result, nil
}

// BusinessProfile returns one full Google Business Profile or null. Each call
// is billed by DataForSEO, including a call that finds no matching profile.
func (c *Client) BusinessProfile(ctx context.Context, req BusinessProfileRequest) (*BusinessProfileResult, error) {
	keyword, err := businessIdentifierKeyword(req.BusinessName, req.CID, req.PlaceID)
	if err != nil {
		return nil, err
	}
	if req.LocationCode < 0 {
		return nil, inputErrorf("locationCode must be positive")
	}
	language, err := c.businessLanguage(req.LanguageCode)
	if err != nil {
		return nil, err
	}
	task := map[string]any{"keyword": keyword, "language_code": language}
	if err := c.setBusinessDataLocation(task, req.Near, req.LocationCode); err != nil {
		return nil, err
	}
	result := &BusinessProfileResult{}
	t, err := c.api.Post(ctx, pathBusinessProfile, task)
	if dataforseo.IsNoResults(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	var response struct {
		Items    []json.RawMessage `json:"items"`
		CheckURL json.RawMessage   `json:"check_url"`
	}
	if _, err := t.FirstResult(&response); err != nil {
		return nil, err
	}
	if len(response.Items) == 0 {
		return result, nil
	}
	result.Profile = businessObject(response.Items[0])
	if result.Profile != nil {
		checkURL := result.Profile["check_url"]
		if len(checkURL) == 0 || string(checkURL) == "null" {
			if len(response.CheckURL) > 0 {
				result.Profile["check_url"] = response.CheckURL
			} else {
				delete(result.Profile, "check_url")
			}
		}
	}
	return result, nil
}

// BusinessQuestions returns answered and unanswered questions with compact
// answer rows. Each call is billed by DataForSEO.
func (c *Client) BusinessQuestions(ctx context.Context, req BusinessQuestionsRequest) (*BusinessQuestionsResult, error) {
	keyword, err := businessIdentifierKeyword(req.BusinessName, req.CID, req.PlaceID)
	if err != nil {
		return nil, err
	}
	if err := validateBusinessSearchNear(req.Near); err != nil {
		return nil, err
	}
	depth, err := businessLimit("depth", req.Depth, 20, 100)
	if err != nil {
		return nil, err
	}
	language, err := c.businessLanguage(req.LanguageCode)
	if err != nil {
		return nil, err
	}
	result := &BusinessQuestionsResult{Questions: []map[string]json.RawMessage{}}
	t, err := c.api.Post(ctx, pathBusinessQuestions, map[string]any{
		"keyword":             keyword,
		"location_coordinate": businessDataCoordinate(*req.Near.Latitude, *req.Near.Longitude, req.Near.RadiusKM),
		"language_code":       language,
		"depth":               depth,
	})
	if dataforseo.IsNoResults(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []json.RawMessage
	if len(t.Result) > 0 {
		if err := json.Unmarshal(t.Result, &entries); err != nil {
			return nil, fmt.Errorf("business questions: decode result: %w", err)
		}
	}
	for _, entry := range entries {
		row := businessObject(entry)
		answered, ok := businessQuestionItems(row["items"])
		if !ok {
			continue
		}
		unanswered, ok := businessQuestionItems(row["items_without_answers"])
		if !ok {
			continue
		}
		for _, question := range append(answered, unanswered...) {
			trimmed := pickBusinessFields(question, businessQuestionFields)
			trimmed["items"] = json.RawMessage("null")
			var answers []json.RawMessage
			if json.Unmarshal(question["items"], &answers) == nil && answers != nil {
				compact := make([]map[string]json.RawMessage, 0, len(answers))
				for _, answer := range answers {
					compact = append(compact, pickBusinessFields(businessObject(answer), businessAnswerFields))
				}
				trimmed["items"], err = json.Marshal(compact)
				if err != nil {
					return nil, err
				}
			}
			result.Questions = append(result.Questions, trimmed)
		}
	}
	return result, nil
}

func businessIdentifierKeyword(name, cid, placeID *string) (string, error) {
	count := 0
	keyword := ""
	for _, id := range []struct {
		field  string
		value  *string
		max    int
		prefix string
	}{
		{"businessName", name, 200, ""}, {"cid", cid, 64, "cid:"}, {"placeId", placeID, 256, "place_id:"},
	} {
		if err := validateBusinessString(id.field, id.value, id.max); err != nil {
			return "", err
		}
		if id.value != nil {
			count++
			keyword = id.prefix + *id.value
		}
	}
	if count != 1 {
		return "", inputErrorf("Provide exactly one business identifier: businessName, cid, or placeId.")
	}
	return keyword, nil
}

// setBusinessDataLocation sets the location of a Google business_data task:
// a coordinate when near is set, otherwise locationCode or the default
// market's location. These endpoints reject a task that has both.
func (c *Client) setBusinessDataLocation(task map[string]any, near *BusinessProfileNear, locationCode int) error {
	if near == nil {
		if locationCode == 0 {
			market, err := c.resolveMarket(0, "")
			if err != nil {
				return err
			}
			locationCode = market.LocationCode
		}
		task["location_code"] = locationCode
		return nil
	}
	if err := validateBusinessCoordinates(near.Latitude, near.Longitude); err != nil {
		return err
	}
	radius := 10.0
	if near.RadiusKM != nil {
		radius = *near.RadiusKM
	}
	if !businessNumberInRange(radius, 0.2, 199) {
		return inputErrorf("near.radiusKm must be between 0.2 and 199")
	}
	task["location_coordinate"] = businessDataCoordinate(*near.Latitude, *near.Longitude, radius)
	return nil
}

func (c *Client) businessLanguage(language string) (string, error) {
	if language == "" {
		market, err := c.resolveMarket(0, "")
		return market.LanguageCode, err
	}
	// Google Business accepts supported languages independently of the
	// client's default country's Labs language list.
	if !supportedLanguages[language] {
		return "", inputErrorf("language code %q is not supported. Use a DataForSEO language code such as 'en', 'es', 'de' or 'fr'.", language)
	}
	return language, nil
}

func validateBusinessString(field string, value *string, maxLength int) error {
	if value != nil {
		if n := researchStringLength(*value); n < 1 || n > maxLength {
			return inputErrorf("%s must contain 1 to %d characters", field, maxLength)
		}
	}
	return nil
}

func businessLimit(field string, value *int, defaultValue, maximum int) (int, error) {
	if value == nil {
		return defaultValue, nil
	}
	if *value < 1 || *value > maximum {
		return 0, inputErrorf("%s must be between 1 and %d", field, maximum)
	}
	return *value, nil
}

func businessNumberInRange(value, low, high float64) bool { return value >= low && value <= high }

func validateBusinessCoordinates(latitude, longitude *float64) error {
	if latitude == nil || !businessNumberInRange(*latitude, -90, 90) {
		return inputErrorf("near.latitude must be between -90 and 90")
	}
	if longitude == nil || !businessNumberInRange(*longitude, -180, 180) {
		return inputErrorf("near.longitude must be between -180 and 180")
	}
	return nil
}

func validateBusinessSearchNear(near *BusinessSearchNear) error {
	if near == nil {
		return inputErrorf("near is required")
	}
	if err := validateBusinessCoordinates(near.Latitude, near.Longitude); err != nil {
		return err
	}
	if !businessNumberInRange(near.RadiusKM, 1, 100000) {
		return inputErrorf("near.radiusKm must be between 1 and 100000")
	}
	return nil
}

func businessCoordinateNumber(value float64) string {
	// FloatString matches toFixed's rounding of exact decimal ties away from
	// zero. Coordinates have already been validated as finite.
	fixed := new(big.Rat).SetFloat64(value).FloatString(7)
	rounded, _ := strconv.ParseFloat(fixed, 64)
	if rounded == 0 {
		return "0"
	}
	if math.Abs(rounded) < 0.000001 {
		return strings.ReplaceAll(strconv.FormatFloat(rounded, 'g', -1, 64), "e-0", "e-")
	}
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

func businessCoordinate(latitude, longitude, radius float64) string {
	return businessCoordinateNumber(latitude) + "," + businessCoordinateNumber(longitude) + "," + strconv.FormatFloat(radius, 'f', 0, 64)
}

func businessDataCoordinate(latitude, longitude, radiusKM float64) string {
	radius := min(199999, max(200, math.Round(radiusKM*1000)))
	return businessCoordinate(latitude, longitude, radius)
}

func businessObject(raw json.RawMessage) map[string]json.RawMessage {
	var row map[string]json.RawMessage
	if json.Unmarshal(raw, &row) != nil {
		return nil
	}
	return row
}

func pickBusinessFields(row map[string]json.RawMessage, fields []string) map[string]json.RawMessage {
	trimmed := make(map[string]json.RawMessage)
	for _, field := range fields {
		if value, ok := row[field]; ok {
			trimmed[field] = value
		}
	}
	return trimmed
}

// The reference validates both question lists together and skips the entire
// result entry if either list contains a value that is not an object.
func businessQuestionItems(raw json.RawMessage) ([]map[string]json.RawMessage, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil, false
	}
	rows := make([]map[string]json.RawMessage, 0, len(items))
	for _, item := range items {
		row := businessObject(item)
		if row == nil {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}
