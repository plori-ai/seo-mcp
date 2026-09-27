package seo

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf16"
)

// RankedKeywordsMarket is the deprecated US-only market selector.
// LocationCode and LanguageCode take precedence over Country.
type RankedKeywordsMarket struct {
	Country string `json:"country,omitempty"`
}

// RankedKeywordsRequest selects a target, market, filters, and result page.
// Nil numeric options use the provider or tool defaults. Scope defaults to
// subdomains for a root target and subfolder for a target with a path.
type RankedKeywordsRequest struct {
	Target            string                `json:"target"`
	Scope             string                `json:"scope,omitempty"`
	Market            *RankedKeywordsMarket `json:"market,omitempty"`
	LocationCode      int                   `json:"locationCode,omitempty"`
	LanguageCode      string                `json:"languageCode,omitempty"`
	ResultTypes       []string              `json:"resultTypes,omitempty"`
	IncludeSubdomains *bool                 `json:"includeSubdomains,omitempty"`
	MinSearchVolume   *int                  `json:"minSearchVolume,omitempty"`
	MaxRank           *int                  `json:"maxRank,omitempty"`
	ExcludeBrandTerms []string              `json:"excludeBrandTerms,omitempty"`
	SortBy            string                `json:"sortBy,omitempty"`
	Limit             *int                  `json:"limit,omitempty"`
	Offset            *int                  `json:"offset,omitempty"`
}

// RankedKeywordsResult contains DataForSEO Labs ranked_keywords items as
// returned by the API, without reshaping their fields.
type RankedKeywordsResult struct {
	Keywords   []json.RawMessage `json:"keywords"`
	TotalCount *float64          `json:"totalCount"`
	Target     string            `json:"target"`
	Scope      string            `json:"scope"`
}

// RankedKeywords returns ranked keyword rows for the requested research scope.
// Each call is billed by DataForSEO.
func (c *Client) RankedKeywords(ctx context.Context, req RankedKeywordsRequest) (*RankedKeywordsResult, error) {
	if err := validateRankedRequest(req); err != nil {
		return nil, err
	}
	target, err := parseResearchTarget(req.Target, "")
	if err != nil {
		return nil, err
	}
	scope := req.Scope
	if scope == "" && req.IncludeSubdomains != nil {
		scope = "domain"
		if *req.IncludeSubdomains {
			scope = "subdomains"
		}
		if target.path != "" {
			scope = "exact_url"
		}
	}
	if scope != "" {
		target, err = parseResearchTarget(req.Target, scope)
		if err != nil {
			return nil, err
		}
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
		return nil, inputErrorf("Domain analytics is not available for this country. Keyword research and rank tracking work; domain-level data is limited to DataForSEO Labs locations.")
	}
	clauses, count := rankedScopeClauses(target)
	if req.MinSearchVolume != nil {
		clauses = append(clauses, []any{"keyword_data.keyword_info.search_volume", ">=", *req.MinSearchVolume})
		count++
	}
	if req.MaxRank != nil {
		clauses = append(clauses, []any{"ranked_serp_element.serp_item.rank_absolute", "<=", *req.MaxRank})
		count++
	}
	for _, term := range req.ExcludeBrandTerms {
		// The reference passes brand-term wildcard characters through unchanged.
		clauses = append(clauses, []any{"keyword_data.keyword", "not_ilike", "%" + term + "%"})
		count++
	}
	if count > 8 {
		return nil, inputErrorf("Too many filter conditions (%d of 8 max).", count)
	}
	limit := 50
	if req.Limit != nil {
		limit = *req.Limit
	}
	order := "keyword_data.keyword_info.search_volume,desc"
	switch req.SortBy {
	case "rank":
		order = "ranked_serp_element.serp_item.rank_absolute,asc"
	case "traffic_estimate":
		order = "ranked_serp_element.serp_item.etv,desc"
	case "cpc":
		order = "keyword_data.keyword_info.cpc,desc"
	}
	payload := map[string]any{"target": target.hostname, "location_code": market.LocationCode, "language_code": market.LanguageCode, "limit": limit, "order_by": []string{order}}
	if req.Offset != nil {
		payload["offset"] = *req.Offset
	}
	if req.ResultTypes != nil {
		payload["item_types"] = req.ResultTypes
	}
	if len(clauses) > 0 {
		payload["filters"] = joinResearchClauses(clauses, "and")
	}
	task, err := c.api.Post(ctx, "/v3/dataforseo_labs/google/ranked_keywords/live", payload)
	if err != nil {
		return nil, fmt.Errorf("ranked keywords: %w", err)
	}
	var result struct {
		Items      []json.RawMessage `json:"items"`
		TotalCount *float64          `json:"total_count"`
	}
	if _, err := task.FirstResult(&result); err != nil {
		return nil, err
	}
	if result.Items == nil {
		result.Items = []json.RawMessage{}
	}
	return &RankedKeywordsResult{Keywords: result.Items, TotalCount: result.TotalCount, Target: target.display, Scope: target.scope}, nil
}

func validateRankedRequest(req RankedKeywordsRequest) error {
	if n := researchStringLength(req.Target); n < 1 || n > 2048 {
		return inputErrorf("target must contain 1 to 2048 characters")
	}
	bare := researchStringLength(req.Target) <= 255 && !strings.HasPrefix(strings.ToLower(req.Target), "www.") && rankedBareDomain.MatchString(req.Target)
	if !bare && !rankedAbsoluteURL.MatchString(req.Target) {
		return inputErrorf("Use a domain without protocol/www or an absolute page URL.")
	}
	if req.Market != nil {
		switch req.Market.Country {
		case "", "US", "USA", "United States", "United States of America":
		default:
			return inputErrorf("market.country must select the United States")
		}
	}
	if req.ResultTypes != nil {
		if len(req.ResultTypes) < 1 || len(req.ResultTypes) > 5 {
			return inputErrorf("resultTypes must contain 1 to 5 entries")
		}
		for _, item := range req.ResultTypes {
			switch item {
			case "organic", "paid", "featured_snippet", "local_pack", "ai_overview_reference":
			default:
				return inputErrorf("Invalid result type: %s", item)
			}
		}
	}
	if req.MinSearchVolume != nil && *req.MinSearchVolume < 0 {
		return inputErrorf("minSearchVolume must be at least 0")
	}
	if req.MaxRank != nil && (*req.MaxRank < 1 || *req.MaxRank > 100) {
		return inputErrorf("maxRank must be between 1 and 100")
	}
	if req.Limit != nil && (*req.Limit < 1 || *req.Limit > 100) {
		return inputErrorf("limit must be between 1 and 100")
	}
	if req.Offset != nil && (*req.Offset < 0 || *req.Offset > 1000) {
		return inputErrorf("offset must be between 0 and 1000")
	}
	if req.ExcludeBrandTerms != nil {
		if len(req.ExcludeBrandTerms) < 1 || len(req.ExcludeBrandTerms) > 10 {
			return inputErrorf("excludeBrandTerms must contain 1 to 10 terms")
		}
		for _, term := range req.ExcludeBrandTerms {
			if n := researchStringLength(term); n < 1 || n > 80 {
				return inputErrorf("Brand terms must contain 1 to 80 characters")
			}
		}
	}
	switch req.SortBy {
	case "", "rank", "search_volume", "traffic_estimate", "cpc":
	default:
		return inputErrorf("Invalid sortBy: %s", req.SortBy)
	}
	return nil
}

// DomainOverviewRequest selects the hostname and market for aggregate metrics.
// Scope controls the display label; metrics always include subdomains.
type DomainOverviewRequest struct {
	Domain            string `json:"domain"`
	Scope             string `json:"scope,omitempty"`
	IncludeSubdomains *bool  `json:"includeSubdomains,omitempty"`
	LocationCode      int    `json:"locationCode,omitempty"`
	LanguageCode      string `json:"languageCode,omitempty"`
}

// DomainOverviewResult is the aggregate organic footprint of a hostname.
// Backlinks and ReferringDomains are null because this Labs endpoint does not
// supply them. FetchedAt is a UTC timestamp with millisecond precision.
type DomainOverviewResult struct {
	Domain           string   `json:"domain"`
	OrganicTraffic   *float64 `json:"organicTraffic"`
	OrganicKeywords  *float64 `json:"organicKeywords"`
	Backlinks        *float64 `json:"backlinks"`
	ReferringDomains *float64 `json:"referringDomains"`
	HasData          bool     `json:"hasData"`
	FetchedAt        string   `json:"fetchedAt"`
	Scope            string   `json:"scope"`
	DisplayTarget    string   `json:"displayTarget"`
}

// DomainOverview returns rounded organic traffic and keyword counts for a
// hostname including its subdomains. Each call is billed by DataForSEO.
func (c *Client) DomainOverview(ctx context.Context, req DomainOverviewRequest) (*DomainOverviewResult, error) {
	if n := researchStringLength(req.Domain); n < 1 || n > 2048 {
		return nil, inputErrorf("domain must contain 1 to 2048 characters")
	}
	market, err := c.resolveMarket(req.LocationCode, req.LanguageCode)
	if err != nil {
		return nil, err
	}
	if adsOnlyLocation(market.LocationCode) {
		return nil, inputErrorf("Domain analytics is not available for this country. Keyword research and rank tracking work; domain-level data is limited to DataForSEO Labs locations.")
	}
	scope := req.Scope
	if scope == "" && req.IncludeSubdomains != nil {
		scope = "domain"
		if *req.IncludeSubdomains {
			scope = "subdomains"
		}
	}
	target, err := parseResearchTarget(req.Domain, scope)
	if err != nil {
		return nil, err
	}
	result := &DomainOverviewResult{Domain: target.hostname, Scope: target.scope, DisplayTarget: target.display, FetchedAt: researchTimestamp(time.Now())}
	task, err := c.api.Post(ctx, "/v3/dataforseo_labs/google/domain_rank_overview/live", map[string]any{"target": target.hostname, "location_code": market.LocationCode, "language_code": market.LanguageCode, "limit": 1})
	if err != nil {
		return nil, fmt.Errorf("domain overview: %w", err)
	}
	var response struct {
		Items []struct {
			Metrics struct {
				Organic struct {
					ETV   *float64 `json:"etv"`
					Count *float64 `json:"count"`
				} `json:"organic"`
			} `json:"metrics"`
		} `json:"items"`
	}
	if _, err := task.FirstResult(&response); err != nil {
		return nil, err
	}
	if len(response.Items) > 0 {
		result.OrganicTraffic = roundResearchMetric(response.Items[0].Metrics.Organic.ETV)
		result.OrganicKeywords = roundResearchMetric(response.Items[0].Metrics.Organic.Count)
	}
	result.HasData = result.OrganicKeywords != nil && *result.OrganicKeywords > 0
	return result, nil
}

func roundResearchMetric(value *float64) *float64 {
	if value == nil {
		return nil
	}
	rounded := math.Floor(*value + 0.5)
	return &rounded
}

func researchTimestamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func researchStringLength(value string) int { return len(utf16.Encode([]rune(value))) }
