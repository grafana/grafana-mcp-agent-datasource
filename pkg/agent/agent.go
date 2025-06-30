package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"grafana-mcpclient-datasource/pkg/models"
)

// Agent orchestrates natural language queries with MCP tools and LLM reasoning
type Agent struct {
	mcpClient   *client.Client
	llmProvider LLMProvider
	logger      log.Logger
}

// LLMProvider interface for different LLM services
type LLMProvider interface {
	GenerateResponse(ctx context.Context, prompt string) (string, error)
	GenerateToolCall(ctx context.Context, query string, tools []mcp.Tool) (*ToolCall, error)
}

// ToolCall represents a decision to call a specific tool with arguments
type ToolCall struct {
	ToolName  string                 `json:"tool_name"`
	Arguments map[string]interface{} `json:"arguments"`
	Reasoning string                 `json:"reasoning"`
}

// QueryResult represents the result of processing a natural language query
type QueryResult struct {
	Query       string       `json:"query"`
	ToolCalls   []ToolCall   `json:"tool_calls"`
	Results     []ToolResult `json:"results"`
	Summary     string       `json:"summary"`
	ProcessedAt time.Time    `json:"processed_at"`
}

// ToolResult represents the result of executing a single tool
type ToolResult struct {
	ToolName string      `json:"tool_name"`
	Success  bool        `json:"success"`
	Data     interface{} `json:"data"`
	Error    string      `json:"error,omitempty"`
}

// NewAgent creates a new agent with the given MCP client and LLM provider
func NewAgent(mcpClient *client.Client, settings models.MCPDataSourceSettings) (*Agent, error) {
	llmProvider, err := createLLMProvider(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM provider: %w", err)
	}

	return &Agent{
		mcpClient:   mcpClient,
		llmProvider: llmProvider,
		logger:      log.DefaultLogger,
	}, nil
}

// ProcessQuery processes a natural language query using available MCP tools
func (a *Agent) ProcessQuery(ctx context.Context, query string) (*QueryResult, error) {
	a.logger.Info("Processing natural language query", "query", query)

	// 1. Get available tools from MCP server
	tools, err := a.getAvailableTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get available tools: %w", err)
	}

	a.logger.Info("Found available tools", "count", len(tools))

	// 2. Use LLM to determine which tools to call
	toolCall, err := a.llmProvider.GenerateToolCall(ctx, query, tools)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tool call: %w", err)
	}

	if toolCall == nil {
		// No tools needed, generate a direct response
		response, err := a.llmProvider.GenerateResponse(ctx, fmt.Sprintf("The user asked: %s\n\nAvailable tools: %s\n\nProvide a helpful response explaining what tools are available.", query, a.formatToolsForPrompt(tools)))
		if err != nil {
			return nil, fmt.Errorf("failed to generate response: %w", err)
		}

		return &QueryResult{
			Query:       query,
			ToolCalls:   []ToolCall{},
			Results:     []ToolResult{},
			Summary:     response,
			ProcessedAt: time.Now(),
		}, nil
	}

	a.logger.Info("Generated tool call", "tool", toolCall.ToolName, "reasoning", toolCall.Reasoning)

	// 3. Execute the selected tool
	toolResult, err := a.executeTool(ctx, *toolCall)
	if err != nil {
		a.logger.Error("Failed to execute tool", "tool", toolCall.ToolName, "error", err)
		toolResult = ToolResult{
			ToolName: toolCall.ToolName,
			Success:  false,
			Error:    err.Error(),
		}
	}

	// 4. Generate a summary of the results
	summary, err := a.generateSummary(ctx, query, []ToolCall{*toolCall}, []ToolResult{toolResult})
	if err != nil {
		a.logger.Warn("Failed to generate summary", "error", err)
		summary = fmt.Sprintf("Executed tool '%s' for query: %s", toolCall.ToolName, query)
	}

	return &QueryResult{
		Query:       query,
		ToolCalls:   []ToolCall{*toolCall},
		Results:     []ToolResult{toolResult},
		Summary:     summary,
		ProcessedAt: time.Now(),
	}, nil
}

// getAvailableTools retrieves the list of available tools from the MCP server
func (a *Agent) getAvailableTools(ctx context.Context) ([]mcp.Tool, error) {
	toolsResponse, err := a.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	return toolsResponse.Tools, nil
}

// executeTool executes a specific tool with the given arguments
func (a *Agent) executeTool(ctx context.Context, toolCall ToolCall) (ToolResult, error) {
	a.logger.Info("Executing tool", "tool", toolCall.ToolName, "args", toolCall.Arguments)

	result, err := a.mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Request: mcp.Request{
			Method: "tools/call",
		},
		Params: mcp.CallToolParams{
			Name:      toolCall.ToolName,
			Arguments: toolCall.Arguments,
		},
	})

	if err != nil {
		return ToolResult{
			ToolName: toolCall.ToolName,
			Success:  false,
			Error:    err.Error(),
		}, err
	}

	// Extract text content from the result
	var resultData interface{}
	if len(result.Content) > 0 {
		textContents := make([]string, 0, len(result.Content))
		for _, content := range result.Content {
			if textContent, ok := mcp.AsTextContent(content); ok {
				textContents = append(textContents, textContent.Text)
			}
		}
		if len(textContents) > 0 {
			resultData = strings.Join(textContents, "\n")
		} else {
			resultData = result.Content
		}
	}

	return ToolResult{
		ToolName: toolCall.ToolName,
		Success:  !result.IsError,
		Data:     resultData,
		Error:    "",
	}, nil
}

// generateSummary creates a human-readable summary of the query processing results
func (a *Agent) generateSummary(ctx context.Context, query string, toolCalls []ToolCall, results []ToolResult) (string, error) {
	prompt := fmt.Sprintf(`
User Query: %s

Tools Called:
%s

Results:
%s

Please provide a clear, concise summary of what was accomplished and the key findings.
`, query, a.formatToolCallsForPrompt(toolCalls), a.formatResultsForPrompt(results))

	return a.llmProvider.GenerateResponse(ctx, prompt)
}

// Helper functions for formatting data for LLM prompts

func (a *Agent) formatToolsForPrompt(tools []mcp.Tool) string {
	var formatted []string
	for _, tool := range tools {
		formatted = append(formatted, fmt.Sprintf("- %s: %s", tool.Name, tool.Description))
	}
	return strings.Join(formatted, "\n")
}

func (a *Agent) formatToolCallsForPrompt(toolCalls []ToolCall) string {
	var formatted []string
	for _, call := range toolCalls {
		argsJSON, _ := json.Marshal(call.Arguments)
		formatted = append(formatted, fmt.Sprintf("- %s (args: %s): %s", call.ToolName, string(argsJSON), call.Reasoning))
	}
	return strings.Join(formatted, "\n")
}

func (a *Agent) formatResultsForPrompt(results []ToolResult) string {
	var formatted []string
	for _, result := range results {
		if result.Success {
			formatted = append(formatted, fmt.Sprintf("- %s: SUCCESS - %v", result.ToolName, result.Data))
		} else {
			formatted = append(formatted, fmt.Sprintf("- %s: ERROR - %s", result.ToolName, result.Error))
		}
	}
	return strings.Join(formatted, "\n")
}

// createLLMProvider creates an appropriate LLM provider based on settings
func createLLMProvider(settings models.MCPDataSourceSettings) (LLMProvider, error) {
	switch strings.ToLower(settings.LLMProvider) {
	case "openai":
		return NewOpenAIProvider(settings.LLMAPIKey, settings.LLMModel)
	case "anthropic":
		return NewAnthropicProvider(settings.LLMAPIKey, settings.LLMModel)
	case "mock", "":
		return NewMockProvider(), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", settings.LLMProvider)
	}
}
