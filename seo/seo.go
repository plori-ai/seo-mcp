// Package seo runs SEO research on the DataForSEO API: keyword research,
// keyword metrics, the keywords a site ranks for, domain and backlink data,
// SERP competitors, and local business data and local search results.
//
// Each operation is a method on Client that takes a request struct and returns
// a result struct. Both carry JSON tags, so a caller can decode tool arguments
// straight into a request and encode a result as a tool response. The field
// names follow the OpenSEO MCP tools (https://github.com/every-app/open-seo),
// from which the research logic is ported.
//
// The package is stateless. It stores nothing, caches nothing, and does no
// billing or authorization; those belong to the caller.
package seo

import (
	"fmt"
	"time"

	"github.com/plori-ai/seo-mcp/dataforseo"
)

// Default market when neither the call nor the Client names one.
const (
	DefaultLocationCode = 2840 // United States
	DefaultLanguageCode = "en"
)

// Client runs research requests against one DataForSEO account. It is safe for
// concurrent use.
type Client struct {
	api          *dataforseo.Client
	market       Market
	taskWait     time.Duration
	taskInterval time.Duration
}

// Market is a DataForSEO location code and language code pair.
type Market struct {
	LocationCode int    `json:"locationCode"`
	LanguageCode string `json:"languageCode"`
}

// Option configures a Client.
type Option func(*Client)

// WithDefaultMarket sets the market used when a request names none.
func WithDefaultMarket(m Market) Option {
	return func(c *Client) { c.market = m }
}

// WithTaskPolling sets how the operations that use DataForSEO task queues
// (BusinessReviews and BusinessUpdates) wait for a result. After a new task is
// posted, the operation checks it every interval until wait has passed; a
// call that collects an earlier task checks it at once and then at the same
// times. When wait ends first, the result has status "processing" and a task
// ID. The defaults are DefaultTaskWait and DefaultTaskPollInterval. A wait of
// zero returns the task ID of a new task without a check. A negative wait
// counts as zero, and an interval that is not positive uses the default.
func WithTaskPolling(wait, interval time.Duration) Option {
	return func(c *Client) {
		c.taskWait = max(wait, 0)
		if interval > 0 {
			c.taskInterval = interval
		}
	}
}

// New returns a Client that sends requests through api.
func New(api *dataforseo.Client, opts ...Option) *Client {
	c := &Client{
		api:          api,
		market:       Market{LocationCode: DefaultLocationCode, LanguageCode: DefaultLanguageCode},
		taskWait:     DefaultTaskWait,
		taskInterval: DefaultTaskPollInterval,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// InputError is a request the caller must change before retrying, such as a
// malformed domain or an unsupported market. Its message says what to change.
type InputError struct{ Msg string }

func (e *InputError) Error() string { return e.Msg }

func inputErrorf(format string, a ...any) error {
	return &InputError{Msg: fmt.Sprintf(format, a...)}
}
