// Package toolset exposes the seo operations as named tools with JSON Schema
// arguments and JSON results. An MCP server, or any agent framework that calls
// tools by name, can serve them without knowing the seo types.
//
// Tool names and argument names match the OpenSEO MCP tools, so a client
// written for OpenSEO can call these tools unchanged.
package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/plori-ai/seo-mcp/seo"
)

// Tool describes one tool.
type Tool struct {
	Name        string
	Title       string
	Description string
	// InputSchema is the JSON Schema of the tool's arguments.
	InputSchema json.RawMessage
	run         func(context.Context, *seo.Client, json.RawMessage) (any, error)
}

// tools lists every tool in the order Tools returns them. Each tool's file
// defines its entry.
var tools = []Tool{
	researchKeywordsTool,
	keywordMetricsTool,
	rankedKeywordsTool,
	domainOverviewTool,
	backlinksOverviewTool,
}

// Tools returns the tool descriptions in a stable order.
func Tools() []Tool {
	out := make([]Tool, len(tools))
	copy(out, tools)
	return out
}

// Set serves the tools with one seo.Client.
type Set struct {
	client *seo.Client
}

// New returns a Set that runs tools with client.
func New(client *seo.Client) *Set {
	return &Set{client: client}
}

// ErrUnknownTool is returned by Call for a name no tool has.
var ErrUnknownTool = errors.New("toolset: unknown tool")

// Call runs the named tool with JSON arguments and returns its result. Invalid
// arguments return a *seo.InputError; DataForSEO failures return an error that
// wraps *dataforseo.Error.
func (s *Set) Call(ctx context.Context, name string, args json.RawMessage) (any, error) {
	for _, t := range tools {
		if t.Name == name {
			return t.run(ctx, s.client, args)
		}
	}
	return nil, fmt.Errorf("%w %q", ErrUnknownTool, name)
}

// bind adapts a typed seo method to the JSON calling convention of Tool.
func bind[Req any, Res any](method func(*seo.Client, context.Context, Req) (Res, error)) func(context.Context, *seo.Client, json.RawMessage) (any, error) {
	return func(ctx context.Context, c *seo.Client, raw json.RawMessage) (any, error) {
		var req Req
		if len(raw) > 0 && string(raw) != "null" {
			if err := json.Unmarshal(raw, &req); err != nil {
				return nil, &seo.InputError{Msg: "invalid arguments: " + err.Error()}
			}
		}
		return method(c, ctx, req)
	}
}
