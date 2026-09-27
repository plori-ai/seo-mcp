package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
	"github.com/plori-ai/seo-mcp/toolset"
)

func newServer(set *toolset.Set) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "seo-mcp", Version: version}, nil)
	for _, definition := range toolset.Tools() {
		openWorld := true
		server.AddTool(&mcp.Tool{
			Name:        definition.Name,
			Title:       definition.Title,
			Description: definition.Description,
			InputSchema: definition.InputSchema,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: &openWorld},
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := set.Call(ctx, definition.Name, req.Params.Arguments)
			if err != nil {
				return toolError(err), nil
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				return toolError(err), nil
			}
			return &mcp.CallToolResult{
				StructuredContent: result,
				Content:           []mcp.Content{&mcp.TextContent{Text: string(encoded)}},
			}, nil
		})
	}
	return server
}

func toolError(err error) *mcp.CallToolResult {
	message := "SEO research failed."
	var input *seo.InputError
	var provider *dataforseo.Error
	switch {
	case errors.As(err, &input):
		message = input.Error()
	case errors.As(err, &provider):
		switch {
		case provider.AuthFailed():
			message = "DataForSEO authentication failed. Check the server credentials."
		case provider.RateLimited():
			message = "DataForSEO rate limited this request. Try again later."
		case provider.Upstream():
			message = "DataForSEO upstream service failed. Try again later."
		case provider.HTTPStatus == 0 && provider.Message != "":
			// A task-level status message is DataForSEO's short reason
			// ("Invalid Field: ...", "Payment Required."); it tells the caller
			// what to change and carries no response body.
			message = "DataForSEO request failed: " + provider.Message
		default:
			message = "DataForSEO request failed."
		}
	case errors.Is(err, context.Canceled):
		message = "SEO research canceled."
	case errors.Is(err, context.DeadlineExceeded):
		message = "SEO research timed out."
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}
