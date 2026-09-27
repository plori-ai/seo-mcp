// Package seo runs SEO research on the DataForSEO API: keyword research,
// keyword metrics, the keywords a site ranks for, a domain overview, and a
// backlink overview.
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
	api    *dataforseo.Client
	market Market
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

// New returns a Client that sends requests through api.
func New(api *dataforseo.Client, opts ...Option) *Client {
	c := &Client{api: api, market: Market{LocationCode: DefaultLocationCode, LanguageCode: DefaultLanguageCode}}
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
