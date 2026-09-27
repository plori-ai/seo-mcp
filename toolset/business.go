package toolset

import (
	"encoding/json"

	"github.com/plori-ai/seo-mcp/seo"
)

var searchLocalBusinessesTool = Tool{
	Name:        "search_local_businesses",
	Title:       "Search local businesses",
	Description: "Returns compact local business listings near a coordinate: identity, categories, contact details, rating, review count, and claimed status. Use it to find nearby competitors, business candidates, or unclaimed listings. Use get_business_profile for one business's full profile. Each call is billed by DataForSEO to the operator's account.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"query":{"type":"string","minLength":1,"maxLength":200,"description":"Business name or title text to match."},
			"near":{"type":"object","properties":{
				"latitude":{"type":"number","minimum":-90,"maximum":90,"description":"Latitude of the search center."},
				"longitude":{"type":"number","minimum":-180,"maximum":180,"description":"Longitude of the search center."},
				"radiusKm":{"type":"number","minimum":1,"maximum":100000,"description":"Search radius in kilometers. Fractions are rounded to whole kilometers."}
			},"required":["latitude","longitude","radiusKm"],"description":"Coordinate and radius to search around."},
			"categories":{"type":"array","minItems":1,"maxItems":10,"items":{"type":"string","minLength":1,"maxLength":120},"description":"Business category slugs, such as pizza_restaurant. Use list_business_categories to find valid slugs."},
			"minRating":{"type":"number","minimum":1,"maximum":5,"description":"Minimum business rating."},
			"minReviews":{"type":"integer","minimum":0,"description":"Minimum Google review count."},
			"isClaimed":{"type":"boolean","description":"Filter by owner claim status. False selects unclaimed listings."},
			"sortBy":{"type":"string","enum":["relevance","rating","reviews"],"description":"Sort order. Defaults to relevance."},
			"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Maximum businesses to return. Defaults to 20."},
			"offset":{"type":"integer","minimum":0,"maximum":1000,"description":"Rows to skip for pagination."}
		},
		"required":["near"]
	}`),
	run: bind((*seo.Client).SearchLocalBusinesses),
}

var listBusinessCategoriesTool = Tool{
	Name:        "list_business_categories",
	Title:       "List business categories",
	Description: "Returns Google Business category slugs and business counts, ranked by business count. Use it to find valid categories for search_local_businesses. Fetches the category list on each call and optionally filters by a case-insensitive substring. DataForSEO does not charge for this call.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"query":{"type":"string","minLength":1,"maxLength":80,"description":"Case-insensitive substring to match against category slugs, such as plumb."},
			"limit":{"type":"integer","minimum":1,"maximum":200,"description":"Maximum categories to return. Defaults to 50."}
		}
	}`),
	run: bind((*seo.Client).ListBusinessCategories),
}

var businessProfileTool = Tool{
	Name:        "get_business_profile",
	Title:       "Get business profile",
	Description: "Returns one full Google Business Profile or null: categories, rating and review count, rating breakdown, address, phone, website, claimed status, opening hours, photos, CID, and place ID. Use it to audit a business profile or compare a competitor's. Supply exactly one of businessName, cid, or placeId. Each call is billed by DataForSEO to the operator's account.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"businessName":{"type":"string","minLength":1,"maxLength":200,"description":"Business name as it appears on Google. Supply exactly one of businessName, cid, or placeId."},
			"cid":{"type":"string","minLength":1,"maxLength":64,"description":"Google business CID, such as one returned by get_local_serp_results."},
			"placeId":{"type":"string","minLength":1,"maxLength":256,"description":"Google Maps place_id, such as one returned by get_local_serp_results."},
			"near":{"type":"object","properties":{
				"latitude":{"type":"number","minimum":-90,"maximum":90,"description":"Latitude of the search center."},
				"longitude":{"type":"number","minimum":-180,"maximum":180,"description":"Longitude of the search center."},
				"radiusKm":{"type":"number","minimum":0.2,"maximum":199,"description":"Search radius in kilometers. Defaults to 10."}
			},"required":["latitude","longitude"],"description":"Search center for an ambiguous business name. Takes precedence over locationCode."},
			"locationCode":{"type":"integer","minimum":1,"description":"DataForSEO location code, including city codes. Defaults to the client's market; ignored when near is set."},
			"languageCode":{"type":"string","description":"Supported language code, such as en, es, de, or fr. Defaults to the client's language even when locationCode is overridden."}
		},
		"oneOf":[{"required":["businessName"]},{"required":["cid"]},{"required":["placeId"]}]
	}`),
	run: bind((*seo.Client).BusinessProfile),
}

var businessQuestionsTool = Tool{
	Name:        "get_google_business_questions",
	Title:       "Get Google business questions",
	Description: "Returns answered and unanswered Google Business Profile questions with question text, author, timestamps, and compact nested answers. Use it when Q&A evidence is needed for a business or FAQ content. Supply exactly one of businessName, cid, or placeId and a search coordinate. Each call is billed by DataForSEO to the operator's account.",
	InputSchema: json.RawMessage(`{
		"type":"object",
		"properties":{
			"businessName":{"type":"string","minLength":1,"maxLength":200,"description":"Business name as it appears on Google. Supply exactly one of businessName, cid, or placeId."},
			"cid":{"type":"string","minLength":1,"maxLength":64,"description":"Google business CID, such as one returned by get_local_serp_results."},
			"placeId":{"type":"string","minLength":1,"maxLength":256,"description":"Google Maps place_id, such as one returned by get_local_serp_results."},
			"near":{"type":"object","properties":{
				"latitude":{"type":"number","minimum":-90,"maximum":90,"description":"Latitude of the search center."},
				"longitude":{"type":"number","minimum":-180,"maximum":180,"description":"Longitude of the search center."},
				"radiusKm":{"type":"number","minimum":1,"maximum":100000,"description":"Radius in kilometers, converted to meters. The provider radius is clamped to 199999 meters."}
			},"required":["latitude","longitude","radiusKm"],"description":"Coordinate and radius to search around."},
			"depth":{"type":"integer","minimum":1,"maximum":100,"description":"Maximum Q&A rows to fetch. Defaults to 20."},
			"languageCode":{"type":"string","description":"Supported language code, such as en, es, de, or fr. Defaults to the client's language."}
		},
		"required":["near"],
		"oneOf":[{"required":["businessName"]},{"required":["cid"]},{"required":["placeId"]}]
	}`),
	run: bind((*seo.Client).BusinessQuestions),
}
