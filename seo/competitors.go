package seo

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// FindSerpCompetitorsMarket is the legacy US-only market selector.
type FindSerpCompetitorsMarket = RankedKeywordsMarket

// FindSerpCompetitorsRequest selects keywords, a Labs market, and a result page.
// Exclusion and sorting apply locally to the page returned by DataForSEO.
type FindSerpCompetitorsRequest struct {
	Keywords          []string                   `json:"keywords"`
	Market            *FindSerpCompetitorsMarket `json:"market,omitempty"`
	LocationCode      int                        `json:"locationCode,omitempty"`
	LanguageCode      string                     `json:"languageCode,omitempty"`
	ResultTypes       []string                   `json:"resultTypes,omitempty"`
	ExcludeDomains    []string                   `json:"excludeDomains,omitempty"`
	IncludeSubdomains *bool                      `json:"includeSubdomains,omitempty"`
	SortBy            string                     `json:"sortBy,omitempty"`
	Limit             *int                       `json:"limit,omitempty"`
	Offset            *int                       `json:"offset,omitempty"`
}

// FindSerpCompetitorsResult preserves all provider fields on each competitor.
type FindSerpCompetitorsResult struct {
	Competitors []json.RawMessage `json:"competitors"`
}

// FindSerpCompetitors returns domains ranking across a keyword set.
// Each call is billed by DataForSEO to the operator's account.
func (c *Client) FindSerpCompetitors(ctx context.Context, req FindSerpCompetitorsRequest) (*FindSerpCompetitorsResult, error) {
	if err := validateSerpCompetitorsRequest(req); err != nil {
		return nil, err
	}
	location, language := req.LocationCode, req.LanguageCode
	if location == 0 && language == "" && req.Market != nil && req.Market.Country != "" {
		location, language = DefaultLocationCode, DefaultLanguageCode
	}
	market, err := c.resolveMarket(location, language)
	if err != nil {
		return nil, err
	}
	if adsOnlyLocation(market.LocationCode) {
		return nil, inputErrorf("SERP competitors are not available for this country. Use a DataForSEO Labs location.")
	}
	limit := 50
	if req.Limit != nil {
		limit = *req.Limit
	}
	itemTypes := req.ResultTypes
	if itemTypes == nil {
		itemTypes = []string{"organic", "local_pack"}
	}
	payload := map[string]any{"keywords": req.Keywords, "location_code": market.LocationCode, "language_code": market.LanguageCode, "item_types": itemTypes, "limit": limit}
	if req.IncludeSubdomains != nil {
		payload["include_subdomains"] = *req.IncludeSubdomains
	}
	if req.Offset != nil {
		payload["offset"] = *req.Offset
	}
	task, err := c.api.Post(ctx, "/v3/dataforseo_labs/google/serp_competitors/live", payload)
	if err != nil {
		return nil, fmt.Errorf("SERP competitors: %w", err)
	}
	var response struct {
		Items []json.RawMessage `json:"items"`
	}
	if _, err := task.FirstResult(&response); err != nil {
		return nil, err
	}
	field := "visibility"
	switch req.SortBy {
	case "avg_position":
		field = "avg_position"
	case "keyword_count":
		field = "keywords_count"
	case "traffic_estimate":
		field = "etv"
	}
	type competitor struct {
		raw   json.RawMessage
		value float64
	}
	rows := make([]competitor, 0, len(response.Items))
	for _, raw := range response.Items {
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			return nil, fmt.Errorf("SERP competitors: decode item: %w", err)
		}
		domain, _ := fields["domain"].(string)
		excluded := slices.ContainsFunc(req.ExcludeDomains, func(excluded string) bool {
			host := strings.ToLower(strings.TrimPrefix(domain, "www."))
			excluded = strings.ToLower(strings.TrimPrefix(excluded, "www."))
			return host == excluded || strings.HasSuffix(host, "."+excluded)
		})
		if !excluded {
			// The reference treats missing and nonnumeric sort values as zero.
			value, _ := fields[field].(float64)
			rows = append(rows, competitor{raw, value})
		}
	}
	slices.SortStableFunc(rows, func(a, b competitor) int {
		if req.SortBy == "avg_position" {
			return cmp.Compare(a.value, b.value)
		}
		return cmp.Compare(b.value, a.value)
	})
	result := &FindSerpCompetitorsResult{Competitors: make([]json.RawMessage, 0, len(rows))}
	for _, row := range rows {
		result.Competitors = append(result.Competitors, row.raw)
	}
	return result, nil
}

func validateSerpCompetitorsRequest(req FindSerpCompetitorsRequest) error {
	if len(req.Keywords) < 1 || len(req.Keywords) > 100 {
		return inputErrorf("keywords must contain 1 to 100 entries")
	}
	for _, keyword := range req.Keywords {
		if n := researchStringLength(keyword); n < 1 || n > 120 {
			return inputErrorf("keywords must contain 1 to 120 characters each")
		}
	}
	if req.Market != nil {
		switch req.Market.Country {
		case "", "US", "USA", "United States", "United States of America":
		default:
			return inputErrorf("market.country must select the United States")
		}
	}
	if req.ResultTypes != nil {
		if len(req.ResultTypes) < 1 || len(req.ResultTypes) > 4 {
			return inputErrorf("resultTypes must contain 1 to 4 entries")
		}
		for _, item := range req.ResultTypes {
			switch item {
			case "organic", "paid", "featured_snippet", "local_pack":
			default:
				return inputErrorf("Invalid result type: %s", item)
			}
		}
	}
	if req.ExcludeDomains != nil {
		if len(req.ExcludeDomains) < 1 || len(req.ExcludeDomains) > 50 {
			return inputErrorf("excludeDomains must contain 1 to 50 entries")
		}
		for _, domain := range req.ExcludeDomains {
			if researchStringLength(domain) > 255 || strings.HasPrefix(strings.ToLower(domain), "www.") || !rankedBareDomain.MatchString(domain) {
				return inputErrorf("Use a domain or subdomain without protocol and without www.")
			}
		}
	}
	if req.Limit != nil && (*req.Limit < 1 || *req.Limit > 100) {
		return inputErrorf("limit must be between 1 and 100")
	}
	if req.Offset != nil && (*req.Offset < 0 || *req.Offset > 1000) {
		return inputErrorf("offset must be between 0 and 1000")
	}
	switch req.SortBy {
	case "", "visibility", "traffic_estimate", "avg_position", "keyword_count":
	default:
		return inputErrorf("Invalid sortBy: %s", req.SortBy)
	}
	return nil
}
