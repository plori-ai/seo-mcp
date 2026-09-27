package seo_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
)

// This example makes a paid DataForSEO request when run with credentials.
// Without an Output comment, go test compiles it but does not execute it.
func ExampleClient_KeywordMetrics() {
	api := dataforseo.New(os.Getenv("DATAFORSEO_API_KEY"))
	client := seo.New(api, seo.WithDefaultMarket(seo.Market{
		LocationCode: 2840,
		LanguageCode: "en",
	}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	result, err := client.KeywordMetrics(ctx, seo.KeywordMetricsRequest{
		Keywords: []string{"technical seo", "site audit"},
		SortBy:   "search_volume",
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}
