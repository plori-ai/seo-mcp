package toolset

import (
	"encoding/json"

	"github.com/plori-ai/seo-mcp/seo"
)

var rankedKeywordsTool = Tool{
	Name:        "get_ranked_keywords",
	Title:       "Get ranked keywords",
	Description: "Returns market-specific keyword, URL, rank, search volume, CPC, intent, and traffic rows for a domain or page. Accepts country-level DataForSEO Labs location/language codes. Use this for keyword strategy evidence; use get_domain_overview for the aggregate domain footprint. Keyword items retain the DataForSEO Labs ranked_keywords API fields. Each call is billed by DataForSEO.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"target":{"type":"string","minLength":1,"maxLength":2048,"anyOf":[{"pattern":"^https?://\\S+$"},{"maxLength":255,"pattern":"^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\\.)+[a-zA-Z]{2,}$","not":{"pattern":"^[wW][wW][wW]\\."}}],"description":"Domain (no protocol/www) or absolute page URL to list ranked keywords for."},
			"scope":{"type":"string","enum":["exact_url","subfolder","domain","subdomains"],"description":"Research scope: 'domain' (hostname without subdomains), 'subdomains' (hostname plus all subdomains), 'subfolder' (path and its children), or 'exact_url' (one page). Defaults to 'subdomains' for root inputs and 'subfolder' when the input has a path."},
			"market":{"type":"object","properties":{"country":{"type":"string","enum":["US","USA","United States","United States of America"],"description":"Country selector. Only the United States can be selected explicitly."}},"description":"Legacy US selector. Prefer locationCode/languageCode for any Labs market. Explicit locationCode or languageCode takes precedence; omission uses the client's default market."},
			"locationCode":{"type":"integer","minimum":1,"description":"Country-level DataForSEO Labs location code. Defaults to the client's market; takes precedence over the legacy market object."},
			"languageCode":{"type":"string","description":"Language for locationCode, such as 'en', 'es', 'de', or 'fr'. Must be served for the selected location. Defaults to that location's primary language when locationCode overrides the client's market."},
			"resultTypes":{"type":"array","minItems":1,"maxItems":5,"items":{"type":"string","enum":["organic","paid","featured_snippet","local_pack","ai_overview_reference"]},"description":"SERP result types to include. Defaults to organic and paid."},
			"includeSubdomains":{"type":"boolean","deprecated":true,"description":"Deprecated: use scope ('subdomains' or 'domain') instead."},
			"minSearchVolume":{"type":"integer","minimum":0,"description":"Only return keywords with at least this monthly search volume."},
			"maxRank":{"type":"integer","minimum":1,"maximum":100,"description":"Only return keywords ranking at this position or better."},
			"excludeBrandTerms":{"type":"array","minItems":1,"maxItems":10,"items":{"type":"string","minLength":1,"maxLength":80},"description":"Exclude keywords containing any of these brand terms."},
			"sortBy":{"type":"string","enum":["rank","search_volume","traffic_estimate","cpc"],"description":"Sort order for returned rows. Defaults to search_volume."},
			"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Maximum rows to return (1-100). Defaults to 50."},
			"offset":{"type":"integer","minimum":0,"maximum":1000,"description":"Rows to skip for pagination."}
		},
		"required":["target"]
	}`),
	run: bind((*seo.Client).RankedKeywords),
}

var domainOverviewTool = Tool{
	Name:        "get_domain_overview",
	Title:       "Get domain overview",
	Description: "Returns a high-level view of a domain's organic footprint: estimated organic traffic, organic keyword count, and data availability. Backlinks and referringDomains are null because this endpoint does not provide backlink counts; use get_backlinks_overview for those. Use this first for domain research, then get_ranked_keywords for detailed keyword rows. Metrics always cover the hostname plus subdomains; scope labels narrower requests. Each call is billed by DataForSEO.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"domain":{"type":"string","minLength":1,"maxLength":2048,"description":"Domain or URL to analyze (e.g. 'example.com')."},
			"scope":{"type":"string","enum":["exact_url","subfolder","domain","subdomains"],"description":"Research scope: 'domain' (hostname without subdomains), 'subdomains' (hostname plus all subdomains), 'subfolder' (path and its children), or 'exact_url' (one page). Defaults to 'subdomains' for root inputs and 'subfolder' when the input has a path. Overview metrics always cover the hostname plus subdomains; narrower scopes are labeled accordingly. Use get_ranked_keywords with a scope for scoped keyword data."},
			"includeSubdomains":{"type":"boolean","deprecated":true,"description":"Deprecated: use scope ('subdomains' or 'domain') instead."},
			"locationCode":{"type":"integer","minimum":1,"description":"Country-level DataForSEO Labs location code. Defaults to the client's market. Domain analytics are unavailable for locations served only by Google Ads."},
			"languageCode":{"type":"string","description":"Supported language code, such as 'en', 'es', 'de', or 'fr'. Must be served for the selected location. Defaults to the client's language or an overriding location's primary language."}
		},
		"required":["domain"]
	}`),
	run: bind((*seo.Client).DomainOverview),
}
