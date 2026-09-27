package toolset

import (
	"encoding/json"

	"github.com/plori-ai/seo-mcp/seo"
)

var backlinksProfileTool = Tool{
	Name:        "get_backlinks_profile",
	Title:       "Get backlinks profile",
	Description: "Returns one page of detailed backlink rows: linking URLs, target URLs, anchors, dofollow status, authority and spam metrics, first/last seen, and lost/broken status. Use it to inspect a target's links or identify referring domains for outreach. Supports filters, sorting, grouping, and pagination. Each call is billed by DataForSEO to the operator's account and requires Backlinks API access.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"target":{"type":"string","minLength":1,"maxLength":2048,"description":"Domain or URL to analyze."},
			"scope":{"type":"string","enum":["exact_url","subfolder","domain","subdomains","page"],"description":"Research scope. Root inputs default to subdomains; paths default to subfolder. Domain excludes subdomains. Exact_url selects one page; page is a deprecated alias. Exact URLs cannot contain query strings or fragments."},
			"page":{"type":"integer","minimum":1,"default":1,"description":"1-indexed results page."},
			"pageSize":{"type":"integer","enum":[50,100,200],"default":100,"description":"Rows per page."},
			"sortField":{"type":"string","enum":["rank","domainRank","spamScore","firstSeen"],"default":"firstSeen","description":"Backlink row sort field."},
			"sortOrder":{"type":"string","enum":["asc","desc"],"default":"desc","description":"Sort direction."},
			"filters":{"type":"object","default":{},"properties":{
				"include":{"type":"string","description":"Source URL terms separated by commas or plus signs; match any term."},
				"exclude":{"type":"string","description":"Source URL terms separated by commas or plus signs; exclude every term."},
				"minDomainRank":{"type":["number","string"],"description":"Minimum domain rank; numeric strings accepted, blank means unset."},
				"maxDomainRank":{"type":["number","string"],"description":"Maximum domain rank; numeric strings accepted, blank means unset."},
				"minLinkAuthority":{"type":["number","string"],"description":"Minimum link rank; numeric strings accepted, blank means unset."},
				"maxLinkAuthority":{"type":["number","string"],"description":"Maximum link rank; numeric strings accepted, blank means unset."},
				"minSpamScore":{"type":["number","string"],"description":"Minimum spam score; numeric strings accepted, blank means unset."},
				"maxSpamScore":{"type":["number","string"],"description":"Maximum spam score; numeric strings accepted, blank means unset."},
				"linkType":{"type":"string","enum":["dofollow","nofollow"]},
				"hideLost":{"type":"boolean"},
				"hideBroken":{"type":"boolean"},
				"domainFrom":{"type":"string","maxLength":255,"description":"Exact linking domain to include."}
			},"description":"Backlink row filters. At most eight conditions total, including four for subfolder scope and one for hideSpam. Numeric ranges have no additional bounds."},
			"mode":{"type":"string","enum":["one_per_domain","as_is"],"default":"one_per_domain","description":"Return each referring domain's strongest link or individual backlink rows."},
			"hideSpam":{"type":"boolean","default":true,"description":"Require backlink spam score at most 40; counts toward the filter limit."}
		},
		"required":["target"]
	}`),
	run: bind((*seo.Client).BacklinksProfile),
}
