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

// Agent represents an intelligent agent that can process natural language queries
// and orchestrate calls to MCP tools
type Agent struct {
	mcpClient   *client.Client
	llmProvider LLMProvider
	settings    models.MCPDataSourceSettings
	logger      log.Logger
}

// LLMProvider interface for different LLM services
type LLMProvider interface {
	GenerateResponse(ctx context.Context, prompt string) (string, error)
	GenerateToolCall(ctx context.Context, query string, tools []mcp.Tool) (*ToolCall, error)
	GenerateStructuredResults(ctx context.Context, query string, toolResults []ToolResult) (*StructuredQueryResult, error)
	FixQuerySyntax(ctx context.Context, originalQuery string, toolName string, errorMessage string, tools []mcp.Tool) (*ToolCall, error)
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

// StructuredQueryResult represents structured data ready for dataframe conversion
type StructuredQueryResult struct {
	Data     []map[string]interface{} `json:"data"`
	Columns  []string                 `json:"columns"`
	Summary  string                   `json:"summary"`
	Metadata map[string]interface{}   `json:"metadata"`
	Query    string                   `json:"query"`
	Success  bool                     `json:"success"`
	ErrorMsg string                   `json:"error_msg,omitempty"`
}

// ToolResult represents the result of executing a single tool
type ToolResult struct {
	ToolName  string                 `json:"tool_name"`
	Success   bool                   `json:"success"`
	Data      interface{}            `json:"data"`
	Error     string                 `json:"error,omitempty"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
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
		settings:    settings,
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

// ProcessQueryStructured processes a natural language query and returns structured results for dataframes
// If toolName is provided, it will use that tool directly instead of using LLM to select one
// If generatedToolCall is provided and matches the current query, it will be used to avoid LLM calls
// If cachedTools is provided, it will be used instead of fetching tools from the server
func (a *Agent) ProcessQueryStructured(ctx context.Context, query string, toolName string, timeRangeFrom, timeRangeTo string, generatedToolCall *models.GeneratedToolCall, cachedTools []mcp.Tool) (*StructuredQueryResult, error) {
	a.logger.Info("Processing natural language query for structured results", "query", query)

	// 1. Get available tools (only if we need them for LLM tool selection)
	var tools []mcp.Tool
	var err error

	// Skip tool fetching if we have a cached generated tool call that matches the query
	needTools := !(generatedToolCall != nil && generatedToolCall.OriginalQuery == query)

	if needTools {
		if len(cachedTools) > 0 {
			tools = cachedTools
			a.logger.Info("Using cached tools", "count", len(tools))
		} else {
			tools, err = a.getAvailableTools(ctx)
			if err != nil {
				return &StructuredQueryResult{
					Query:    query,
					Success:  false,
					ErrorMsg: fmt.Sprintf("Failed to get available tools: %v", err),
				}, nil
			}
			a.logger.Info("Fetched tools from server", "count", len(tools))
		}
	} else {
		a.logger.Info("Skipping tool fetching - using cached generated tool call")
	}

	// 2. Determine which tool to use
	maxRetries := a.settings.GetAgentRetries()
	var toolCall *ToolCall
	var toolResult ToolResult
	var lastError string
	var actualAttempts int

	// Check if we have a cached generated tool call that matches the current query
	if generatedToolCall != nil && generatedToolCall.OriginalQuery == query {
		a.logger.Info("Using cached generated tool call", "toolName", generatedToolCall.ToolName, "query", query)
		toolCall = &ToolCall{
			ToolName:  generatedToolCall.ToolName,
			Arguments: generatedToolCall.Arguments,
			Reasoning: "Using cached tool call from previous execution",
		}
		maxRetries = 1 // Skip retry loop since we're using cached tool call
	} else if toolName != "" {
		a.logger.Info("Using user-selected tool", "toolName", toolName)

		// Validate that the selected tool exists
		var selectedTool *mcp.Tool
		for _, tool := range tools {
			if tool.Name == toolName {
				selectedTool = &tool
				break
			}
		}

		if selectedTool == nil {
			return &StructuredQueryResult{
				Query:    query,
				Success:  false,
				ErrorMsg: fmt.Sprintf("Selected tool '%s' is not available. Available tools: %v", toolName, a.getToolNames(tools)),
			}, nil
		}

		// Create tool call with user-selected tool and use LLM to generate arguments
		toolCall = &ToolCall{
			ToolName:  toolName,
			Arguments: make(map[string]interface{}),
			Reasoning: "Tool was explicitly selected by user",
		}

		// Use LLM to generate appropriate arguments for the selected tool
		var argumentsPrompt string
		if timeRangeFrom != "" && timeRangeTo != "" {
			argumentsPrompt = fmt.Sprintf(`You are an intelligent agent. The user asked: "%s"

The user has selected the tool: %s
Tool description: %s

Dashboard time range:
- From: %s
- To: %s

Generate appropriate arguments for this tool based on the user's query and the provided time range. Include time range parameters where applicable. Respond with a JSON object containing just the arguments.

For example, if this is a log query tool, you might generate:
{"query": "{level=\"error\"}", "limit": 100, "start": "%s", "end": "%s"}

Arguments JSON:`, query, selectedTool.Name, selectedTool.Description, timeRangeFrom, timeRangeTo, timeRangeFrom, timeRangeTo)
		} else {
			argumentsPrompt = fmt.Sprintf(`You are an intelligent agent. The user asked: "%s"

The user has selected the tool: %s
Tool description: %s

Generate appropriate arguments for this tool based on the user's query. Respond with a JSON object containing just the arguments.

For example, if this is a log query tool, you might generate:
{"query": "{level=\"error\"}", "limit": 100}

Arguments JSON:`, query, selectedTool.Name, selectedTool.Description)
		}

		response, err := a.llmProvider.GenerateResponse(ctx, argumentsPrompt)
		if err == nil {
			// Try to parse the arguments from LLM response
			var args map[string]interface{}
			start := strings.Index(response, "{")
			end := strings.LastIndex(response, "}") + 1
			if start >= 0 && end > start {
				jsonStr := response[start:end]
				if json.Unmarshal([]byte(jsonStr), &args) == nil {
					toolCall.Arguments = args
				}
			}
		}

		// Skip retry loop since we're using user-selected tool
		maxRetries = 1
	}

	for attempt := 1; attempt <= maxRetries; attempt++ {
		actualAttempts = attempt // Track actual attempts made
		a.logger.Info("Query processing attempt", "attempt", attempt, "maxRetries", maxRetries)

		// Generate or fix tool call (only if not user-selected)
		if toolName == "" {
			if attempt == 1 {
				// First attempt: normal tool call generation using LLM
				// Create enhanced query with time range context if available
				enhancedQuery := query
				if timeRangeFrom != "" && timeRangeTo != "" {
					enhancedQuery = fmt.Sprintf(`%s from %s to %s`, query, timeRangeFrom, timeRangeTo)
				}

				toolCall, err = a.llmProvider.GenerateToolCall(ctx, enhancedQuery, tools)
				if err != nil {
					return &StructuredQueryResult{
						Query:    query,
						Success:  false,
						ErrorMsg: fmt.Sprintf("Failed to generate tool call: %v", err),
					}, nil
				}
			} else {
				// Retry attempt: ask LLM to fix the syntax error
				a.logger.Info("Attempting to fix syntax error", "attempt", attempt, "lastError", lastError)
				toolCall, err = a.llmProvider.FixQuerySyntax(ctx, query, toolCall.ToolName, lastError, tools)
				if err != nil {
					a.logger.Error("Failed to fix syntax error", "attempt", attempt, "error", err)
					continue // Try next attempt or give up
				}
			}
		}

		if toolCall == nil {
			// No tools needed, return info about available tools
			return &StructuredQueryResult{
				Query:   query,
				Data:    []map[string]interface{}{},
				Columns: []string{},
				Summary: fmt.Sprintf("No specific tools needed for this query. Available tools: %s", a.formatToolsForPrompt(tools)),
				Success: true,
				Metadata: map[string]interface{}{
					"tool_count":     len(tools),
					"processed_at":   time.Now(),
					"tools_executed": 0,
					"attempts":       attempt,
				},
			}, nil
		}

		a.logger.Info("Generated tool call", "tool", toolCall.ToolName, "reasoning", toolCall.Reasoning, "attempt", attempt)

		// 3. Execute the selected tool
		toolResult, err = a.executeTool(ctx, *toolCall)
		if err != nil {
			a.logger.Error("Failed to execute tool", "tool", toolCall.ToolName, "error", err, "attempt", attempt)
			toolResult = ToolResult{
				ToolName:  toolCall.ToolName,
				Success:   false,
				Error:     err.Error(),
				Arguments: toolCall.Arguments,
			}
		}

		// 4. Check if the execution was successful or if it's a syntax error that can be retried
		if toolResult.Success {
			a.logger.Info("Tool execution successful", "tool", toolCall.ToolName, "attempt", attempt)
			break // Success! Exit retry loop
		}

		// Check if this is a syntax error that we can retry
		errorMsg := strings.ToLower(toolResult.Error)
		isSyntaxError := strings.Contains(errorMsg, "syntax") ||
			strings.Contains(errorMsg, "parse") ||
			strings.Contains(errorMsg, "invalid") ||
			strings.Contains(errorMsg, "unexpected") ||
			strings.Contains(errorMsg, "malformed")

		if !isSyntaxError || attempt >= maxRetries {
			// Not a syntax error or max retries reached, stop retrying
			a.logger.Info("Stopping retry loop", "isSyntaxError", isSyntaxError, "attempt", attempt, "maxRetries", maxRetries)
			break
		}

		// Store error for next retry attempt
		lastError = toolResult.Error
		a.logger.Info("Detected syntax error, will retry", "attempt", attempt, "error", lastError)
	}

	// 5. Generate structured results using LLM
	structuredResult, err := a.llmProvider.GenerateStructuredResults(ctx, query, []ToolResult{toolResult})
	if err != nil {
		return &StructuredQueryResult{
			Query:    query,
			Success:  false,
			ErrorMsg: fmt.Sprintf("Failed to generate structured results: %v", err),
		}, nil
	}

	// Add additional metadata
	if structuredResult.Metadata == nil {
		structuredResult.Metadata = make(map[string]interface{})
	}
	structuredResult.Metadata["tool_name"] = toolCall.ToolName
	structuredResult.Metadata["tool_reasoning"] = toolCall.Reasoning
	structuredResult.Metadata["tool_success"] = toolResult.Success
	structuredResult.Metadata["retry_attempts"] = maxRetries
	structuredResult.Metadata["attempts_made"] = actualAttempts
	structuredResult.Metadata["arguments"] = toolCall.Arguments

	// Store the generated tool call for future cache use (only if not using cached or user-selected tool)
	if generatedToolCall == nil && toolName == "" {
		structuredResult.Metadata["generated_tool_call"] = map[string]interface{}{
			"toolName":      toolCall.ToolName,
			"arguments":     toolCall.Arguments,
			"originalQuery": query,
		}
	}

	if !toolResult.Success {
		structuredResult.Metadata["tool_error"] = toolResult.Error
		if lastError != "" {
			structuredResult.Metadata["final_error_after_retries"] = true
		}
	}

	return structuredResult, nil
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
			ToolName:  toolCall.ToolName,
			Success:   false,
			Error:     err.Error(),
			Arguments: toolCall.Arguments,
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
		ToolName:  toolCall.ToolName,
		Success:   !result.IsError,
		Data:      resultData,
		Error:     "",
		Arguments: toolCall.Arguments,
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

// getToolNames extracts tool names from a slice of tools for error messages
func (a *Agent) getToolNames(tools []mcp.Tool) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

// createLLMProvider creates an appropriate LLM provider based on settings
func createLLMProvider(settings models.MCPDataSourceSettings) (LLMProvider, error) {
	switch strings.ToLower(settings.LLMProvider) {
	case "openai":
		return NewOpenAIProvider(settings)
	case "anthropic":
		return NewAnthropicProvider(settings)
	case "mock", "":
		return NewMockProvider(settings), nil
	default:
		return nil, fmt.Errorf("unsupported LLM provider: %s", settings.LLMProvider)
	}
}
