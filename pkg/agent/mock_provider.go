package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// MockProvider is a simple mock implementation for testing
type MockProvider struct{}

// NewMockProvider creates a new mock LLM provider
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

// GenerateResponse generates a mock response
func (m *MockProvider) GenerateResponse(ctx context.Context, prompt string) (string, error) {
	// Simple mock response based on prompt content
	if strings.Contains(strings.ToLower(prompt), "error") {
		return "I encountered an error while processing your request. Please check the tool execution results for more details.", nil
	}

	if strings.Contains(strings.ToLower(prompt), "tools") {
		return "I can help you with various tasks using the available tools. The tools can query data, analyze logs, and provide insights based on your requests.", nil
	}

	return "I've processed your request using the available tools. The results show the information you requested.", nil
}

// GenerateToolCall analyzes the query and decides which tool to call
func (m *MockProvider) GenerateToolCall(ctx context.Context, query string, tools []mcp.Tool) (*ToolCall, error) {
	if len(tools) == 0 {
		return nil, nil // No tools available
	}

	// Simple mock logic for tool selection
	queryLower := strings.ToLower(query)

	for _, tool := range tools {
		toolNameLower := strings.ToLower(tool.Name)

		// If query mentions logs, search, or query, and we have a loki_query tool
		if (strings.Contains(queryLower, "log") ||
			strings.Contains(queryLower, "search") ||
			strings.Contains(queryLower, "query") ||
			strings.Contains(queryLower, "find") ||
			strings.Contains(queryLower, "error")) &&
			strings.Contains(toolNameLower, "loki") {

			// Extract a simple query pattern
			var logQuery string
			if strings.Contains(queryLower, "error") {
				logQuery = `{level="error"}`
			} else if strings.Contains(queryLower, "warn") {
				logQuery = `{level="warn"}`
			} else {
				logQuery = `{job=~".+"}`
			}

			return &ToolCall{
				ToolName: tool.Name,
				Arguments: map[string]interface{}{
					"query": logQuery,
					"limit": 100,
				},
				Reasoning: fmt.Sprintf("The user's query '%s' appears to be asking for log data, so I'll use the %s tool to search for relevant logs.", query, tool.Name),
			}, nil
		}
	}

	// If no specific tool matches, use the first available tool with generic arguments
	firstTool := tools[0]
	return &ToolCall{
		ToolName:  firstTool.Name,
		Arguments: map[string]interface{}{},
		Reasoning: fmt.Sprintf("I'll use the %s tool to help answer your query: %s", firstTool.Name, query),
	}, nil
}
