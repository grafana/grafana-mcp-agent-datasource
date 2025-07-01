package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"grafana-mcpclient-datasource/pkg/models"

	"github.com/mark3labs/mcp-go/mcp"
)

// MockProvider provides a simple mock implementation for testing
type MockProvider struct {
	settings models.MCPDataSourceSettings
}

// NewMockProvider creates a new mock provider
func NewMockProvider(settings models.MCPDataSourceSettings) *MockProvider {
	return &MockProvider{
		settings: settings,
	}
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

// GenerateStructuredResults generates structured data from tool results
func (m *MockProvider) GenerateStructuredResults(ctx context.Context, query string, toolResults []ToolResult) (*StructuredQueryResult, error) {
	// Mock implementation that structures the results appropriately

	// Check if we have successful results
	var data []map[string]interface{}
	var summary string
	var success bool = true
	var errorMsg string

	for _, result := range toolResults {
		if result.Success {
			// For Loki queries, create structured log entries
			if result.ToolName == "loki_query" {
				if dataStr, ok := result.Data.(string); ok {
					// Parse the log data and create structured entries
					// For mock purposes, create sample structured data
					data = append(data, map[string]interface{}{
						"timestamp": time.Now().Format(time.RFC3339),
						"level":     "error",
						"message":   dataStr,
						"service":   "mock-service",
						"tool":      result.ToolName,
					})
				}
			} else {
				// For other tools, create a generic structure
				data = append(data, map[string]interface{}{
					"tool":      result.ToolName,
					"result":    result.Data,
					"timestamp": time.Now().Format(time.RFC3339),
				})
			}
		} else {
			success = false
			errorMsg = result.Error
		}
	}

	// Generate appropriate summary
	if len(toolResults) == 0 {
		summary = "No tools were executed for this query."
	} else if success {
		summary = fmt.Sprintf("Successfully executed %d tool(s) and found %d result(s).", len(toolResults), len(data))
	} else {
		summary = fmt.Sprintf("Tool execution failed: %s", errorMsg)
	}

	// Define columns based on data structure
	columns := []string{"timestamp", "level", "message", "service", "tool"}
	if len(data) > 0 {
		// Extract columns from first data entry
		columns = make([]string, 0, len(data[0]))
		for key := range data[0] {
			columns = append(columns, key)
		}
	}

	return &StructuredQueryResult{
		Data:     data,
		Columns:  columns,
		Summary:  summary,
		Query:    query,
		Success:  success,
		ErrorMsg: errorMsg,
		Metadata: map[string]interface{}{
			"tool_count":   len(toolResults),
			"processed_at": time.Now(),
			"provider":     "mock",
		},
	}, nil
}

// FixQuerySyntax generates a corrected tool call based on syntax error feedback (mock implementation)
func (m *MockProvider) FixQuerySyntax(ctx context.Context, originalQuery string, toolName string, errorMessage string, tools []mcp.Tool) (*ToolCall, error) {
	// Mock implementation that fixes common LogQL syntax errors

	// Simple mock logic for fixing syntax errors
	var fixedQuery string
	var reasoning string

	queryLower := strings.ToLower(originalQuery)

	if strings.Contains(strings.ToLower(errorMessage), "syntax") || strings.Contains(strings.ToLower(errorMessage), "parse") {
		// Common LogQL fixes
		if strings.Contains(queryLower, "error") {
			fixedQuery = `{level="error"}`
			reasoning = "Fixed syntax by providing a simple error level selector in proper LogQL format"
		} else if strings.Contains(queryLower, "warn") {
			fixedQuery = `{level="warn"}`
			reasoning = "Fixed syntax by providing a simple warning level selector in proper LogQL format"
		} else {
			fixedQuery = `{job=~".+"}`
			reasoning = "Fixed syntax by providing a basic job selector that matches all jobs"
		}
	} else {
		// Default fallback
		fixedQuery = `{job=~".+"} | limit 100`
		reasoning = "Applied default fix with basic selector and limit"
	}

	return &ToolCall{
		ToolName: toolName,
		Arguments: map[string]interface{}{
			"query": fixedQuery,
			"limit": 100,
		},
		Reasoning: fmt.Sprintf("Mock fix applied: %s. Original error: %s", reasoning, errorMessage),
	}, nil
}
