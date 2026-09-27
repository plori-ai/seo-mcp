package toolset

import (
	"encoding/json"

	"github.com/plori-ai/seo-mcp/seo"
)

var researchKeywordsTool = Tool{
	Name:        "research_keywords",
	Title:       "Research keywords",
	Description: "Finds related keywords for 1-5 seed keywords in one call and returns, per keyword, search volume, keyword difficulty (0-100), CPC in USD, paid competition (0-1), search intent, and a monthly search-volume trend. Use it to discover keywords around a topic; use get_keyword_metrics to score keywords you already have. Returns one result per seed; a seed that fails has ok false and an error, and the other seeds still return. Each seed is billed by DataForSEO: one DataForSEO Labs related_keywords request, plus keyword_suggestions and keyword_ideas requests when related_keywords returns fewer than 5 keywords other than the seed (source and usedFallback say which ran). Countries served from Google Ads data (for example Iceland, 2352) use one flat-priced Google Ads request per seed and return no keyword difficulty or intent.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"seeds":{"type":"array","minItems":1,"maxItems":5,"items":{
				"type":"object",
				"properties":{
					"seed":{"type":"string","minLength":1,"description":"Seed keyword to research."},
					"locationCode":{"type":"integer","minimum":1,"description":"DataForSEO country location code, for example 2840 (United States) or 2276 (Germany); see https://dataforseo.com/help-center/locations. Defaults to the client's market. Some countries (for example Iceland, 2352) are served from Google Ads data: volume, CPC and trends work, but keyword difficulty and intent are unavailable."},
					"languageCode":{"type":"string","description":"Language code, such as 'en', 'es', 'de' or 'vi'. Must be served for the location. Defaults to the client's language when the location is the client's location, otherwise to the location's primary language."}
				},
				"required":["seed"]
			},"description":"1-5 seed keywords. Each seed is researched independently and returns related keywords with volume, difficulty and CPC. Prefer one call with several seeds over several single-seed calls."},
			"resultLimit":{"type":"integer","enum":[150,300,500],"description":"Maximum keywords returned per seed. Defaults to 150."},
			"includeClickstreamData":{"type":"boolean","description":"Refine search volumes with clickstream data, which splits Google Ads' grouped close-variant volumes (plurals, misspellings). Doubles the DataForSEO price of each seed. Defaults to false. No effect for countries served from Google Ads data."}
		},
		"required":["seeds"]
	}`),
	run: bind((*seo.Client).ResearchKeywords),
}

var keywordMetricsTool = Tool{
	Name:        "get_keyword_metrics",
	Title:       "Get keyword metrics",
	Description: "Returns search volume, keyword difficulty (0-100), search intent, CPC in USD, paid competition (0-1 and LOW/MEDIUM/HIGH), and monthly search volumes for up to 700 known keywords in one call. Use it to score candidate or known keywords, such as Search Console queries, by demand and ranking difficulty; use research_keywords to find new keywords. Keywords DataForSEO has no data for are left out of the result. For countries served from Google Ads data (for example Iceland, 2352), keyword difficulty and intent are null. Each call is one DataForSEO request, billed by DataForSEO.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"keywords":{"type":"array","minItems":1,"maxItems":700,"items":{"type":"string","minLength":1,"maxLength":80},"description":"Keywords to fetch metrics for (1-700)."},
			"locationCode":{"type":"integer","minimum":1,"description":"DataForSEO country location code, for example 2840 (United States) or 2276 (Germany); see https://dataforseo.com/help-center/locations. Defaults to the client's market. Some countries (for example Iceland, 2352) are served from Google Ads data: volume, CPC and trends work, but keyword difficulty and intent are unavailable."},
			"languageCode":{"type":"string","description":"Language code, such as 'en', 'es', 'de' or 'vi'. Must be served for the location. Defaults to the client's language when the location is the client's location, otherwise to the location's primary language."},
			"includeMonthlyTrends":{"type":"boolean","description":"Include the monthly_searches rows. Defaults to true."},
			"includeClickstreamData":{"type":"boolean","description":"Refine search volumes with clickstream data, which splits Google Ads' grouped close-variant volumes (plurals, misspellings). Doubles the DataForSEO price of the call. Defaults to false. No effect for countries served from Google Ads data."},
			"sortBy":{"type":"string","enum":["search_volume","keyword_difficulty","cpc","competition"],"description":"Field to sort rows by, highest first. Defaults to search_volume."}
		},
		"required":["keywords"]
	}`),
	run: bind((*seo.Client).KeywordMetrics),
}
