package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/plori-ai/seo-mcp/dataforseo"
	"github.com/plori-ai/seo-mcp/seo"
	"github.com/plori-ai/seo-mcp/toolset"
)

func newServer(set *toolset.Set) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "seo-mcp", Version: buildVersion()}, nil)
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
	message := errorMessage(err)
	var task *seo.TaskError
	if errors.As(err, &task) {
		// The task is paid for; without its ID the caller can only post and
		// pay for a new one.
		message += fmt.Sprintf(" DataForSEO created the task before the failure. Call this tool again with taskId %q to collect it at no extra charge.", task.TaskID)
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

func errorMessage(err error) string {
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
		case provider.HTTPStatus >= 500 && strings.HasSuffix(provider.Path, "/task_post"):
			// The transport does not replay a task_post, because DataForSEO
			// may have created and billed the task before the error.
			message = "DataForSEO failed while it created the task. It may have created and billed the task anyway. Wait a few minutes before you try again, because a new call creates a new billed task."
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
	return message
}
