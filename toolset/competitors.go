package toolset

import (
	"encoding/json"

	"github.com/plori-ai/seo-mcp/seo"
)

var findSerpCompetitorsTool = Tool{
	Name:        "find_serp_competitors",
	Title:       "Find SERP competitors",
	Description: "Returns domains competing in Google results across a supplied keyword set, including keyword counts, positions, visibility, and estimated traffic. Use it to discover competitors in a country-level DataForSEO Labs market. Exclusions and sorting apply to the fetched page, so exclusions may reduce the returned count. Each call is billed by DataForSEO to the operator's account.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"keywords":{"type":"array","minItems":1,"maxItems":100,"items":{"type":"string","minLength":1,"maxLength":120},"description":"Keywords whose SERPs are compared."},
			"market":{"type":"object","properties":{"country":{"type":"string","enum":["US","USA","United States","United States of America"]}},"description":"Legacy US selector. Explicit locationCode or languageCode takes precedence; omission uses the client's default market."},
			"locationCode":{"type":"integer","minimum":1,"description":"Country-level DataForSEO Labs location code. Defaults to the client's market."},
			"languageCode":{"type":"string","description":"Language served for locationCode. Defaults to the client's language or an overriding location's primary language."},
			"resultTypes":{"type":"array","minItems":1,"maxItems":4,"items":{"type":"string","enum":["organic","paid","featured_snippet","local_pack"]},"description":"SERP result types to include. Defaults to organic and local_pack."},
			"excludeDomains":{"type":"array","minItems":1,"maxItems":50,"items":{"type":"string","minLength":1,"maxLength":255,"pattern":"^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\\.)+[a-zA-Z]{2,}$","not":{"pattern":"^[wW][wW][wW]\\."}},"description":"Bare domains or subdomains without protocol or www. Excludes each domain and its subdomains locally."},
			"includeSubdomains":{"type":"boolean","description":"Count subdomains as part of the same competitor domain. Omission uses the provider default."},
			"sortBy":{"type":"string","enum":["visibility","traffic_estimate","avg_position","keyword_count"],"description":"Local sort order. Defaults to visibility descending; avg_position sorts ascending."},
			"limit":{"type":"integer","minimum":1,"maximum":100,"description":"Maximum competitors to fetch. Defaults to 50."},
			"offset":{"type":"integer","minimum":0,"maximum":1000,"description":"Provider rows to skip for pagination."}
		},
		"required":["keywords"]
	}`),
	run: bind((*seo.Client).FindSerpCompetitors),
}
