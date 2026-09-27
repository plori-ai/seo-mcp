package toolset

import (
	"encoding/json"

	"github.com/plori-ai/seo-mcp/seo"
)

var backlinksOverviewTool = Tool{
	Name:        "get_backlinks_overview",
	Title:       "Get backlinks overview",
	Description: "Returns a backlinks profile summary, historical trends, and up to 100 top referring domains. Use it to assess a target's backlink footprint and linking domains. Bare domains default to scope 'subdomains'; pass scope 'domain' to exclude subdomains from summary totals. Targets with a path default to 'subfolder', whose counts come from filtered backlink totals, with no rank, trends, or referring-domain breakdown. Trend data always includes subdomains and is unavailable for exact URLs. Each call is billed by DataForSEO and requires the Backlinks API enabled on your DataForSEO account.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"target":{"type":"string","minLength":1,"description":"Domain or URL to analyze (e.g. 'example.com' or 'https://example.com/blog')."},
			"scope":{"type":"string","enum":["exact_url","subfolder","domain","subdomains","page"],"description":"Research scope: 'domain' (hostname without subdomains), 'subdomains' (hostname plus all subdomains), 'subfolder' (path and its children), or 'exact_url' (one page). Defaults to 'subdomains' for root inputs and 'subfolder' when the input has a path. 'page' is a deprecated alias of 'exact_url'. Subfolder counts are computed from filtered backlink totals; rank, trends, and the referring-domains breakdown are unavailable for subfolders."},
			"hideSpam":{"type":"boolean","description":"Filter out spammy referring domains. Defaults to true. Does not change summary or trend metrics; subfolder counts always use the default spam filter."}
		},
		"required":["target"]
	}`),
	run: bind((*seo.Client).BacklinksOverview),
}
