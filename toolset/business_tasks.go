package toolset

import (
	"encoding/json"

	"github.com/plori-ai/seo-mcp/seo"
)

// The identifier and location properties are those of get_business_profile.
var businessReviewsTool = Tool{
	Name:        "get_business_reviews",
	Title:       "Get business reviews",
	Description: "Collects Google reviews for a business, with rating, author, text, and the owner's reply. Use it for review-gap analysis against competitors and to find unanswered reviews. DataForSEO runs this as a queued task. The call usually returns the reviews; if the task is still running, the result has status \"processing\" and a taskId. Call again with only that taskId after 30 to 60 seconds to collect the result at no extra charge. Supply exactly one of businessName, cid, or placeId unless you pass taskId. Each new task is billed by DataForSEO to the operator's account.",
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
			"languageCode":{"type":"string","description":"Supported language code, such as en, es, de, or fr. Defaults to the client's language."},
			"depth":{"type":"integer","minimum":10,"maximum":200,"description":"Number of reviews to collect. Defaults to 20. Billed per 10 reviews, or per 20 when includeOtherSources is true."},
			"sortBy":{"type":"string","enum":["newest","highest_rating","lowest_rating","relevant"],"description":"Review sort order. Defaults to newest. Ignored when includeOtherSources is true, because that endpoint cannot sort."},
			"includeOtherSources":{"type":"boolean","description":"Also collect the reviews Google shows from other sites, such as Yelp, Tripadvisor, and Trustpilot. Defaults to false. Costs more per review and cannot be sorted."},
			"taskId":{"type":"string","minLength":1,"maxLength":128,"description":"Collect a task that an earlier call returned with status \"processing\". Pass the taskId exactly as returned, in the form google:<id> or extended:<id>. The call then posts no new task, charges nothing, and ignores the other arguments."}
		}
	}`),
	run: bind((*seo.Client).BusinessReviews),
}

var businessUpdatesTool = Tool{
	Name:        "get_business_updates",
	Title:       "Get business updates",
	Description: "Collects the posts (updates, offers, and events) published on a Google Business Profile. Use it to check how often and how recently a business posts. DataForSEO runs this as a queued task. The call usually returns the posts; if the task is still running, the result has status \"processing\" and a taskId. Call again with only that taskId after 30 to 60 seconds to collect the result at no extra charge. Supply exactly one of businessName, cid, or placeId unless you pass taskId. Each new task is billed by DataForSEO to the operator's account.",
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
			"languageCode":{"type":"string","description":"Supported language code, such as en, es, de, or fr. Defaults to the client's language."},
			"depth":{"type":"integer","minimum":10,"maximum":100,"description":"Number of posts to collect. Defaults to 10. Billed per 10 posts."},
			"taskId":{"type":"string","minLength":1,"maxLength":128,"description":"Collect a task that an earlier call returned with status \"processing\". Pass the taskId exactly as returned. The call then posts no new task, charges nothing, and ignores the other arguments."}
		}
	}`),
	run: bind((*seo.Client).BusinessUpdates),
}
