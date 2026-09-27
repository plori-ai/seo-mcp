package toolset

import (
	"encoding/json"

	"github.com/plori-ai/seo-mcp/seo"
)

var localSerpResultsTool = Tool{
	Name:        "get_local_serp_results",
	Title:       "Get local SERP results",
	Description: "Returns one Google Maps or Local Finder SERP near a coordinate, with business identity, rank, rating, categories, contact details, and hours. Use it to inspect nearby rankings and obtain a CID or place ID for business research or a local rank grid. Each call makes one request billed by DataForSEO to the operator's account.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"keyword":{"type":"string","minLength":1,"maxLength":120,"description":"Search query to run on Google Maps or Local Finder."},
			"near":{"type":"object","properties":{
				"latitude":{"type":"number","minimum":-90,"maximum":90,"description":"Latitude the SERP is fetched from."},
				"longitude":{"type":"number","minimum":-180,"maximum":180,"description":"Longitude the SERP is fetched from."},
				"zoom":{"type":"integer","minimum":4,"maximum":18,"description":"Optional map zoom. Higher zoom narrows the local area."}
			},"required":["latitude","longitude"],"description":"Coordinate and optional zoom the SERP is fetched from."},
			"searchType":{"type":"string","enum":["maps","local_finder"],"description":"Which local SERP to fetch. Defaults to maps."},
			"device":{"type":"string","enum":["desktop","mobile"],"description":"Device the SERP is rendered for. Defaults to mobile."},
			"depth":{"type":"integer","minimum":1,"maximum":100,"description":"Number of results to fetch (1-100). Defaults to 20."},
			"languageCode":{"type":"string","description":"Supported SERP language code, such as 'en', 'es', 'de', or 'fr'. Defaults to the client's market language."}
		},
		"required":["keyword","near"]
	}`),
	run: bind((*seo.Client).LocalSerpResults),
}

var localRankGridTool = Tool{
	Name:        "get_local_rank_grid",
	Title:       "Get local rank grid",
	Description: "Returns a square Google Maps ranking grid around a coordinate, with each point's target rank, result count, and first business, plus coverage and average rank. Use it to measure how a business's Maps visibility varies around its storefront. Each call makes gridSize squared DataForSEO requests at depth 20: gridSize 3 (the default) makes 9 requests; gridSize 5 makes 25. Requests run at most three at a time; cancellation or an HTTP authentication failure can stop remaining requests. Each call is billed by DataForSEO to the operator's account, including point searches that fail but are charged.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"keyword":{"type":"string","minLength":1,"maxLength":120,"description":"Search query to run on Google Maps at every grid point."},
			"target":{"type":"object","properties":{
				"cid":{"type":"string","minLength":1,"maxLength":64,"description":"Match a row with this Google business CID."},
				"placeId":{"type":"string","minLength":1,"maxLength":256,"description":"Match a row with this Google Maps place_id."},
				"name":{"type":"string","minLength":1,"maxLength":200,"description":"Match a row whose title contains this text, case-insensitively, when its identifiers do not match."}
			},"anyOf":[{"required":["cid"]},{"required":["placeId"]},{"required":["name"]}],"description":"The business to locate. Supply at least one of cid, placeId, or name. The first matching row supplies the rank."},
			"center":{"type":"object","properties":{
				"latitude":{"type":"number","minimum":-90,"maximum":90,"description":"Latitude of the center."},
				"longitude":{"type":"number","minimum":-180,"maximum":180,"description":"Longitude of the center."}
			},"required":["latitude","longitude"],"description":"Coordinate the grid is centered on, usually the storefront."},
			"gridSize":{"type":"integer","enum":[3,5],"description":"Grid width: 3 (9 requests) or 5 (25 requests). Defaults to 3."},
			"spacingKm":{"type":"number","minimum":0.25,"maximum":10,"description":"Distance between neighboring points in kilometers. Defaults to 2."},
			"device":{"type":"string","enum":["desktop","mobile"],"description":"Device the SERP is rendered for. Defaults to mobile."},
			"zoom":{"type":"integer","minimum":4,"maximum":18,"description":"Shared map zoom. Defaults to a zoom derived from spacingKm and center latitude so each point's viewport spans the grid spacing."},
			"languageCode":{"type":"string","description":"Supported SERP language code, such as 'en', 'es', 'de', or 'fr'. Defaults to the client's market language."}
		},
		"required":["keyword","target","center"]
	}`),
	run: bind((*seo.Client).LocalRankGrid),
}
