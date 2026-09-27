package toolset_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
	"github.com/plori-ai/seo-mcp/toolset"
)

// This example makes a paid DataForSEO request when run with credentials.
// Without an Output comment, go test compiles it but does not execute it.
func ExampleSet_Call() {
	api := dataforseo.New(os.Getenv("DATAFORSEO_API_KEY"))
	set := toolset.New(seo.New(api))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := set.Call(ctx, "get_ranked_keywords", json.RawMessage(`{
		"target": "example.com",
		"scope": "subdomains",
		"locationCode": 2840,
		"languageCode": "en",
		"limit": 10
	}`))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

func ExampleTools() {
	for _, tool := range toolset.Tools() {
		fmt.Println(tool.Name, tool.Title)
	}
	// Output:
	// research_keywords Research keywords
	// get_keyword_metrics Get keyword metrics
	// get_ranked_keywords Get ranked keywords
	// get_domain_overview Get domain overview
	// get_backlinks_overview Get backlinks overview
	// find_serp_competitors Find SERP competitors
	// get_backlinks_profile Get backlinks profile
	// search_local_businesses Search local businesses
	// list_business_categories List business categories
	// get_business_profile Get business profile
	// get_google_business_questions Get Google business questions
	// get_business_reviews Get business reviews
	// get_business_updates Get business updates
	// get_local_serp_results Get local SERP results
	// get_local_rank_grid Get local rank grid
}
