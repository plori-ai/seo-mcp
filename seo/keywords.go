package seo

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

const (
	pathRelatedKeywords        = "/v3/dataforseo_labs/google/related_keywords/live"
	pathKeywordSuggestions     = "/v3/dataforseo_labs/google/keyword_suggestions/live"
	pathKeywordIdeas           = "/v3/dataforseo_labs/google/keyword_ideas/live"
	pathKeywordOverview        = "/v3/dataforseo_labs/google/keyword_overview/live"
	pathAdsKeywordsForKeywords = "/v3/keywords_data/google_ads/keywords_for_keywords/live"
	pathAdsSearchVolume        = "/v3/keywords_data/google_ads/search_volume/live"
)

// Keyword sources reported in SeedResult.Source.
const (
	sourceRelated     = "related"
	sourceSuggestions = "suggestions"
	sourceIdeas       = "ideas"
	sourceGoogleAds   = "google_ads"
)

// autoSources is the order in which ResearchKeywords tries the DataForSEO Labs
// sources. A later source runs only when the earlier ones together returned
// fewer than minNonSeedKeywords keywords other than the seed.
var autoSources = []string{sourceRelated, sourceSuggestions, sourceIdeas}

const (
	maxSeeds           = 5
	defaultResultLimit = 150
	minNonSeedKeywords = 5
	// relatedDepth is the related_keywords search depth: keywords up to three
	// hops away from the seed in Google's "searches related to" graph.
	relatedDepth = 3

	maxMetricsKeywords = 700 // DataForSEO's limit per keyword_overview or search_volume task
	maxKeywordLength   = 80
)

// Sort orders accepted by KeywordMetricsRequest.SortBy.
var metricsSortFields = []string{"search_volume", "keyword_difficulty", "cpc", "competition"}

// ResearchKeywordsRequest is the input of ResearchKeywords.
type ResearchKeywordsRequest struct {
	// Seeds lists 1 to 5 seed keywords. Each seed is researched independently.
	Seeds []KeywordSeed `json:"seeds"`
	// ResultLimit is the maximum number of keywords returned per seed: 150,
	// 300 or 500. Zero means 150.
	ResultLimit int `json:"resultLimit,omitempty"`
	// IncludeClickstreamData asks DataForSEO Labs for search volumes refined
	// with clickstream data. It doubles the DataForSEO price of each request
	// and has no effect in countries served from Google Ads data.
	IncludeClickstreamData bool `json:"includeClickstreamData,omitempty"`
}

// KeywordSeed is one seed keyword and its market. A zero LocationCode or an
// empty LanguageCode selects the default as described on Client.
type KeywordSeed struct {
	Seed         string `json:"seed"`
	LocationCode int    `json:"locationCode,omitempty"`
	LanguageCode string `json:"languageCode,omitempty"`
}

// ResearchKeywordsResult holds one entry per seed, in request order.
type ResearchKeywordsResult struct {
	Results []SeedResult `json:"results"`
}

// SeedResult is the research result of one seed. A seed that failed has OK
// false and Error set, and the other seeds of the request are unaffected. In
// JSON, a failed seed carries only seed, ok and error.
type SeedResult struct {
	// Seed is the seed as given in the request.
	Seed string `json:"seed"`
	OK   bool   `json:"ok"`
	// RowCount is len(Rows).
	RowCount int `json:"rowCount"`
	// Source is the DataForSEO source of the last request made for the seed:
	// "related" (related_keywords), "suggestions" (keyword_suggestions),
	// "ideas" (keyword_ideas) or "google_ads" (Google Ads keywords_for_keywords,
	// used for countries DataForSEO Labs does not cover).
	Source string `json:"source,omitempty"`
	// UsedFallback reports that related_keywords returned too few keywords and
	// later sources were queried.
	UsedFallback bool         `json:"usedFallback"`
	Rows         []KeywordRow `json:"rows"`
	Error        string       `json:"error,omitempty"`
}

// MarshalJSON encodes a failed seed as {seed, ok, error} and a successful one
// with all fields, rows as an array even when empty.
func (r SeedResult) MarshalJSON() ([]byte, error) {
	if !r.OK {
		return json.Marshal(struct {
			Seed  string `json:"seed"`
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}{r.Seed, false, r.Error})
	}
	type plain SeedResult // drops the method, so Marshal does not recurse
	p := plain(r)
	if p.Rows == nil {
		p.Rows = []KeywordRow{}
	}
	return json.Marshal(p)
}

// KeywordRow is one researched keyword. Nil numbers are values DataForSEO did
// not return.
type KeywordRow struct {
	// Keyword is trimmed and lowercased.
	Keyword      string       `json:"keyword"`
	SearchVolume *int64       `json:"searchVolume"`
	Trend        []TrendMonth `json:"trend"`
	// CPC is the cost per click in USD.
	CPC *float64 `json:"cpc"`
	// Competition is the paid-search competition, from 0 to 1.
	Competition *float64 `json:"competition"`
	// KeywordDifficulty is from 0 to 100. It is nil for countries served from
	// Google Ads data.
	KeywordDifficulty *int `json:"keywordDifficulty"`
	// Intent is "informational", "commercial", "transactional",
	// "navigational" or "unknown".
	Intent string `json:"intent"`
}

// TrendMonth is the search volume of one month.
type TrendMonth struct {
	Year         int   `json:"year"`
	Month        int   `json:"month"`
	SearchVolume int64 `json:"searchVolume"`
}

// KeywordMetricsRequest is the input of KeywordMetrics.
type KeywordMetricsRequest struct {
	// Keywords lists 1 to 700 keywords of at most 80 characters each.
	Keywords []string `json:"keywords"`
	// LocationCode and LanguageCode select the market; zero values select the
	// default as described on Client.
	LocationCode int    `json:"locationCode,omitempty"`
	LanguageCode string `json:"languageCode,omitempty"`
	// IncludeMonthlyTrends controls the monthly_searches field. Nil means true.
	IncludeMonthlyTrends *bool `json:"includeMonthlyTrends,omitempty"`
	// SortBy orders the rows by this field, highest first: "search_volume"
	// (the default), "keyword_difficulty", "cpc" or "competition".
	SortBy string `json:"sortBy,omitempty"`
	// IncludeClickstreamData asks DataForSEO Labs for search volumes refined
	// with clickstream data. It doubles the DataForSEO price and has no effect
	// in countries served from Google Ads data.
	IncludeClickstreamData bool `json:"includeClickstreamData,omitempty"`
}

// KeywordMetricsResult holds the metrics of the keywords DataForSEO returned
// data for. Keywords it has no data for are absent.
type KeywordMetricsResult struct {
	Keywords []KeywordMetrics `json:"keywords"`
}

// KeywordMetrics is the metrics of one keyword. Nil values are values
// DataForSEO did not return.
type KeywordMetrics struct {
	// Keyword is the keyword as DataForSEO returned it.
	Keyword      string `json:"keyword"`
	SearchVolume *int64 `json:"search_volume"`
	// KeywordDifficulty is from 0 to 100. It is nil for countries served from
	// Google Ads data.
	KeywordDifficulty *int `json:"keyword_difficulty"`
	// MainIntent is DataForSEO's search intent label. It is nil for countries
	// served from Google Ads data.
	MainIntent *string `json:"main_intent"`
	// CPC is the cost per click in USD.
	CPC *float64 `json:"cpc"`
	// Competition is the paid-search competition, from 0 to 1.
	Competition *float64 `json:"competition"`
	// CompetitionLevel is "LOW", "MEDIUM" or "HIGH".
	CompetitionLevel *string `json:"competition_level"`
	// MonthlySearches is nil when the request set IncludeMonthlyTrends to
	// false, and the field is then omitted from JSON. When trends are
	// requested and DataForSEO returned none, it points to a nil slice, which
	// encodes as null.
	MonthlySearches *[]MonthlySearch `json:"monthly_searches,omitempty"`
}

// MonthlySearch is the search volume of one month.
type MonthlySearch struct {
	Year         int   `json:"year"`
	Month        int   `json:"month"`
	SearchVolume int64 `json:"search_volume"`
}

// ResearchKeywords returns related keywords with search volume, difficulty,
// CPC, competition, intent and a monthly trend for each seed. Seeds run
// concurrently. Each seed costs one DataForSEO request, or up to three when
// the first source returns too few keywords (see SeedResult.Source). A seed
// that fails is reported in its SeedResult; the method returns an error only
// for invalid input or a cancelled context.
func (c *Client) ResearchKeywords(ctx context.Context, req ResearchKeywordsRequest) (*ResearchKeywordsResult, error) {
	if len(req.Seeds) == 0 || len(req.Seeds) > maxSeeds {
		return nil, inputErrorf("seeds must list 1 to %d seed keywords; got %d.", maxSeeds, len(req.Seeds))
	}
	for i, s := range req.Seeds {
		if strings.TrimSpace(s.Seed) == "" {
			return nil, inputErrorf("seeds[%d].seed is empty; give a keyword.", i)
		}
	}
	limit := req.ResultLimit
	switch limit {
	case 0:
		limit = defaultResultLimit
	case 150, 300, 500:
	default:
		return nil, inputErrorf("resultLimit must be 150, 300 or 500; got %d.", limit)
	}

	results := make([]SeedResult, len(req.Seeds))
	var wg sync.WaitGroup
	for i, s := range req.Seeds {
		wg.Go(func() {
			rows, source, usedFallback, err := c.researchSeed(ctx, s, limit, req.IncludeClickstreamData)
			if err != nil {
				results[i] = SeedResult{Seed: s.Seed, Error: err.Error()}
				return
			}
			results[i] = SeedResult{
				Seed:         s.Seed,
				OK:           true,
				RowCount:     len(rows),
				Source:       source,
				UsedFallback: usedFallback,
				Rows:         rows,
			}
		})
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &ResearchKeywordsResult{Results: results}, nil
}

// researchSeed researches one seed. In a DataForSEO Labs country it queries
// related_keywords, then keyword_suggestions, then keyword_ideas, and stops at
// the first source after which the merged rows hold enough keywords other
// than the seed. A DataForSEO error fails the seed; it does not move on to the
// next source.
func (c *Client) researchSeed(ctx context.Context, s KeywordSeed, limit int, clickstream bool) ([]KeywordRow, string, bool, error) {
	m, err := c.resolveMarket(s.LocationCode, s.LanguageCode)
	if err != nil {
		return nil, "", false, err
	}
	seed := normalizeKeyword(s.Seed)

	if adsOnlyLocation(m.LocationCode) {
		rows, err := c.adsKeywordIdeas(ctx, seed, m, limit)
		return rows, sourceGoogleAds, false, err
	}

	merged := make([]KeywordRow, 0, limit)
	seen := make(map[string]bool)
	for i, source := range autoSources {
		rows, err := c.labsKeywordRows(ctx, source, seed, m, limit, clickstream)
		if err != nil {
			return nil, "", false, err
		}
		for _, row := range rows {
			if len(merged) >= limit {
				break
			}
			if seen[row.Keyword] {
				continue
			}
			seen[row.Keyword] = true
			merged = append(merged, row)
		}
		if countNonSeed(merged, seed) >= minNonSeedKeywords {
			return merged, source, i > 0, nil
		}
	}
	return merged, sourceIdeas, true, nil
}

// labsKeywordRows fetches the keywords of one DataForSEO Labs source.
func (c *Client) labsKeywordRows(ctx context.Context, source, seed string, m Market, limit int, clickstream bool) ([]KeywordRow, error) {
	task := map[string]any{
		"location_code":            m.LocationCode,
		"language_code":            m.LanguageCode,
		"limit":                    limit,
		"include_clickstream_data": clickstream,
		"include_serp_info":        false,
	}
	var path string
	switch source {
	case sourceRelated:
		path = pathRelatedKeywords
		task["keyword"] = seed
		task["depth"] = relatedDepth
	case sourceSuggestions:
		path = pathKeywordSuggestions
		task["keyword"] = seed
		task["include_seed_keyword"] = true
		task["ignore_synonyms"] = false
		task["exact_match"] = false
	default:
		path = pathKeywordIdeas
		task["keywords"] = []string{seed}
		task["ignore_synonyms"] = false
		task["closely_variants"] = false
	}
	t, err := c.api.Post(ctx, path, task)
	if err != nil {
		return nil, err
	}

	if source == sourceRelated {
		// related_keywords wraps each keyword one level deeper than the
		// other two sources.
		var res struct {
			Items []struct {
				KeywordData *labsKeywordItem `json:"keyword_data"`
			} `json:"items"`
		}
		if _, err := t.FirstResult(&res); err != nil {
			return nil, err
		}
		items := make([]labsKeywordItem, 0, len(res.Items))
		for _, it := range res.Items {
			if it.KeywordData != nil {
				items = append(items, *it.KeywordData)
			}
		}
		return researchRowsFromLabs(items), nil
	}

	var res struct {
		Items []labsKeywordItem `json:"items"`
	}
	if _, err := t.FirstResult(&res); err != nil {
		return nil, err
	}
	return researchRowsFromLabs(res.Items), nil
}

// adsKeywordIdeas fetches keyword ideas from Google Ads, for countries
// DataForSEO Labs does not cover.
func (c *Client) adsKeywordIdeas(ctx context.Context, seed string, m Market, limit int) ([]KeywordRow, error) {
	t, err := c.api.Post(ctx, pathAdsKeywordsForKeywords, map[string]any{
		"keywords":      []string{seed},
		"location_code": m.LocationCode,
		"language_code": m.LanguageCode,
		"sort_by":       "search_volume",
	})
	if err != nil {
		return nil, err
	}
	items, err := adsItems(t, pathAdsKeywordsForKeywords)
	if err != nil {
		return nil, err
	}
	// The endpoint has no limit parameter and returns all ideas for one flat
	// price, so the limit is applied here.
	if len(items) > limit {
		items = items[:limit]
	}

	rows := make([]KeywordRow, 0, len(items))
	seen := make(map[string]bool)
	for _, it := range items {
		if it.Keyword == "" {
			continue
		}
		kw := normalizeKeyword(it.Keyword)
		if seen[kw] {
			continue
		}
		seen[kw] = true
		rows = append(rows, KeywordRow{
			Keyword:      kw,
			SearchVolume: it.SearchVolume,
			Trend:        trendMonths(it.MonthlySearches),
			CPC:          it.CPC,
			Competition:  it.competitionRatio(),
			Intent:       "unknown",
		})
	}
	return rows, nil
}

// researchRowsFromLabs maps Labs keyword items to rows, dropping items without
// a keyword and repeated keywords.
func researchRowsFromLabs(items []labsKeywordItem) []KeywordRow {
	rows := make([]KeywordRow, 0, len(items))
	seen := make(map[string]bool)
	for _, it := range items {
		if it.Keyword == "" {
			continue
		}
		kw := normalizeKeyword(it.Keyword)
		if seen[kw] {
			continue
		}
		seen[kw] = true

		// The clickstream block is present only when the request asked for
		// clickstream data. A zero volume in it falls back to keyword_info,
		// as OpenSEO does.
		volume := it.KeywordInfo
		if cs := it.Clickstream; cs != nil && cs.SearchVolume != nil && *cs.SearchVolume != 0 {
			volume = cs
		}
		row := KeywordRow{
			Keyword:           kw,
			Trend:             []TrendMonth{},
			KeywordDifficulty: it.keywordDifficulty(),
			Intent:            normalizeIntent(it.mainIntent()),
		}
		if volume != nil {
			row.SearchVolume = volume.SearchVolume
			row.Trend = trendMonths(volume.MonthlySearches)
		}
		if it.KeywordInfo != nil {
			row.CPC = it.KeywordInfo.CPC
			row.Competition = it.KeywordInfo.Competition
		}
		rows = append(rows, row)
	}
	return rows
}

// KeywordMetrics returns search volume, keyword difficulty, intent, CPC,
// competition and monthly search volumes for a list of known keywords, in one
// DataForSEO request. Keywords DataForSEO has no data for are left out of the
// result.
func (c *Client) KeywordMetrics(ctx context.Context, req KeywordMetricsRequest) (*KeywordMetricsResult, error) {
	if len(req.Keywords) == 0 || len(req.Keywords) > maxMetricsKeywords {
		return nil, inputErrorf("keywords must list 1 to %d keywords; got %d.", maxMetricsKeywords, len(req.Keywords))
	}
	for i, k := range req.Keywords {
		if k == "" || utf8.RuneCountInString(k) > maxKeywordLength {
			return nil, inputErrorf("keywords[%d] must be 1 to %d characters long.", i, maxKeywordLength)
		}
	}
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "search_volume"
	}
	if !slices.Contains(metricsSortFields, sortBy) {
		return nil, inputErrorf("sortBy must be one of %s; got %q.", strings.Join(metricsSortFields, ", "), req.SortBy)
	}
	m, err := c.resolveMarket(req.LocationCode, req.LanguageCode)
	if err != nil {
		return nil, err
	}

	var rows []KeywordMetrics
	if adsOnlyLocation(m.LocationCode) {
		t, err := c.api.Post(ctx, pathAdsSearchVolume, map[string]any{
			"keywords":      req.Keywords,
			"location_code": m.LocationCode,
			"language_code": m.LanguageCode,
		})
		if err != nil {
			return nil, err
		}
		items, err := adsItems(t, pathAdsSearchVolume)
		if err != nil {
			return nil, err
		}
		rows = metricsFromAds(items)
	} else {
		t, err := c.api.Post(ctx, pathKeywordOverview, map[string]any{
			"keywords":                 req.Keywords,
			"location_code":            m.LocationCode,
			"language_code":            m.LanguageCode,
			"include_clickstream_data": req.IncludeClickstreamData,
		})
		if err != nil {
			return nil, err
		}
		var res struct {
			Items []labsKeywordItem `json:"items"`
		}
		if _, err := t.FirstResult(&res); err != nil {
			return nil, err
		}
		rows = metricsFromLabs(res.Items)
	}

	slices.SortStableFunc(rows, func(a, b KeywordMetrics) int {
		return cmp.Compare(b.sortValue(sortBy), a.sortValue(sortBy)) // highest first
	})
	if req.IncludeMonthlyTrends != nil && !*req.IncludeMonthlyTrends {
		for i := range rows {
			rows[i].MonthlySearches = nil
		}
	}
	return &KeywordMetricsResult{Keywords: rows}, nil
}

// sortValue returns the field named by sortBy, with a missing value as 0.
func (k KeywordMetrics) sortValue(sortBy string) float64 {
	switch sortBy {
	case "keyword_difficulty":
		if k.KeywordDifficulty != nil {
			return float64(*k.KeywordDifficulty)
		}
	case "cpc":
		if k.CPC != nil {
			return *k.CPC
		}
	case "competition":
		if k.Competition != nil {
			return *k.Competition
		}
	default:
		if k.SearchVolume != nil {
			return float64(*k.SearchVolume)
		}
	}
	return 0
}

func metricsFromLabs(items []labsKeywordItem) []KeywordMetrics {
	rows := make([]KeywordMetrics, 0, len(items))
	for _, it := range items {
		if it.Keyword == "" {
			continue
		}
		// Unlike research, a clickstream volume of zero is used as is.
		volume := it.KeywordInfo
		if cs := it.Clickstream; cs != nil && cs.SearchVolume != nil {
			volume = cs
		}
		row := KeywordMetrics{
			Keyword:           it.Keyword,
			KeywordDifficulty: it.keywordDifficulty(),
			MainIntent:        it.mainIntent(),
		}
		var monthly []MonthlySearch
		if volume != nil {
			row.SearchVolume = volume.SearchVolume
			monthly = monthlySearches(volume.MonthlySearches)
		}
		row.MonthlySearches = &monthly
		if info := it.KeywordInfo; info != nil {
			row.CPC = info.CPC
			row.Competition = info.Competition
			row.CompetitionLevel = info.CompetitionLevel
		}
		rows = append(rows, row)
	}
	return rows
}

func metricsFromAds(items []adsKeywordItem) []KeywordMetrics {
	rows := make([]KeywordMetrics, 0, len(items))
	for _, it := range items {
		if it.Keyword == "" {
			continue
		}
		monthly := monthlySearches(it.MonthlySearches)
		rows = append(rows, KeywordMetrics{
			Keyword:          it.Keyword,
			SearchVolume:     it.SearchVolume,
			CPC:              it.CPC,
			Competition:      it.competitionRatio(),
			CompetitionLevel: it.Competition,
			MonthlySearches:  &monthly,
		})
	}
	return rows
}

// labsKeywordItem is the keyword data DataForSEO Labs returns per keyword.
type labsKeywordItem struct {
	Keyword     string       `json:"keyword"`
	KeywordInfo *keywordInfo `json:"keyword_info"`
	// Clickstream is present only when the request set
	// include_clickstream_data.
	Clickstream       *keywordInfo `json:"keyword_info_normalized_with_clickstream"`
	KeywordProperties *struct {
		KeywordDifficulty *int `json:"keyword_difficulty"`
	} `json:"keyword_properties"`
	SearchIntentInfo *struct {
		MainIntent *string `json:"main_intent"`
	} `json:"search_intent_info"`
}

func (it labsKeywordItem) keywordDifficulty() *int {
	if it.KeywordProperties == nil {
		return nil
	}
	return it.KeywordProperties.KeywordDifficulty
}

func (it labsKeywordItem) mainIntent() *string {
	if it.SearchIntentInfo == nil {
		return nil
	}
	return it.SearchIntentInfo.MainIntent
}

type keywordInfo struct {
	SearchVolume *int64   `json:"search_volume"`
	CPC          *float64 `json:"cpc"`
	// Competition is a 0-1 ratio in Labs.
	Competition      *float64      `json:"competition"`
	CompetitionLevel *string       `json:"competition_level"`
	MonthlySearches  []wireMonthly `json:"monthly_searches"`
}

// adsKeywordItem is the keyword data the Google Ads endpoints return.
type adsKeywordItem struct {
	Keyword      string   `json:"keyword"`
	SearchVolume *int64   `json:"search_volume"`
	CPC          *float64 `json:"cpc"`
	// Competition is "LOW", "MEDIUM" or "HIGH".
	Competition *string `json:"competition"`
	// CompetitionIndex is the competition on a 0-100 scale.
	CompetitionIndex *float64      `json:"competition_index"`
	MonthlySearches  []wireMonthly `json:"monthly_searches"`
}

// competitionRatio converts the 0-100 competition index to the 0-1 ratio Labs
// reports, so both sources return the same scale.
func (it adsKeywordItem) competitionRatio() *float64 {
	if it.CompetitionIndex == nil {
		return nil
	}
	v := *it.CompetitionIndex / 100
	return &v
}

type wireMonthly struct {
	Year         *int   `json:"year"`
	Month        *int   `json:"month"`
	SearchVolume *int64 `json:"search_volume"`
}

// adsItems decodes a Keywords Data task, whose result array holds the keyword
// items directly instead of under result[0].items as in Labs.
func adsItems(t *dataforseo.Task, path string) ([]adsKeywordItem, error) {
	if len(t.Result) == 0 || string(t.Result) == "null" {
		return nil, nil
	}
	var items []adsKeywordItem
	if err := json.Unmarshal(t.Result, &items); err != nil {
		return nil, fmt.Errorf("dataforseo: decode result from %s: %w", path, err)
	}
	return items, nil
}

func trendMonths(in []wireMonthly) []TrendMonth {
	out := make([]TrendMonth, 0, len(in))
	for _, m := range in {
		out = append(out, TrendMonth{Year: valueOrZero(m.Year), Month: valueOrZero(m.Month), SearchVolume: valueOrZero(m.SearchVolume)})
	}
	return out
}

// monthlySearches returns nil for no entries, which KeywordMetrics encodes as
// null.
func monthlySearches(in []wireMonthly) []MonthlySearch {
	if len(in) == 0 {
		return nil
	}
	out := make([]MonthlySearch, 0, len(in))
	for _, m := range in {
		out = append(out, MonthlySearch{Year: valueOrZero(m.Year), Month: valueOrZero(m.Month), SearchVolume: valueOrZero(m.SearchVolume)})
	}
	return out
}

func valueOrZero[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func normalizeKeyword(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normalizeIntent maps DataForSEO's main_intent label to one of the five
// intent values of KeywordRow.
func normalizeIntent(raw *string) string {
	if raw == nil {
		return "unknown"
	}
	v := strings.ToLower(*raw)
	switch {
	case strings.Contains(v, "inform"):
		return "informational"
	case strings.Contains(v, "commerc"):
		return "commercial"
	case strings.Contains(v, "transact"):
		return "transactional"
	case strings.Contains(v, "navig"):
		return "navigational"
	}
	return "unknown"
}

func countNonSeed(rows []KeywordRow, seed string) int {
	n := 0
	for _, r := range rows {
		if r.Keyword != seed {
			n++
		}
	}
	return n
}
