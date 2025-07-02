package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"grafana-mcpclient-datasource/pkg/models"

	"github.com/mark3labs/mcp-go/mcp"
)

// AnthropicProvider implements LLM functionality using Anthropic's Claude API
type AnthropicProvider struct {
	apiKey   string
	model    string
	baseURL  string
	settings models.MCPDataSourceSettings
}

// AnthropicRequest represents the request structure for Claude API
type AnthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
	System    string    `json:"system,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// AnthropicResponse represents the response from Claude API
type AnthropicResponse struct {
	Content []Content `json:"content"`
	Usage   Usage     `json:"usage"`
}

// Content represents the content in the response
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Usage represents token usage information
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(settings models.MCPDataSourceSettings) (*AnthropicProvider, error) {
	if settings.LLMAPIKey == "" {
		return nil, fmt.Errorf("Anthropic API key is required")
	}

	model := settings.LLMModel
	if model == "" {
		model = "claude-3-5-sonnet-20241022" // Default model
	}

	return &AnthropicProvider{
		apiKey:   settings.LLMAPIKey,
		model:    model,
		baseURL:  "https://api.anthropic.com/v1/messages",
		settings: settings,
	}, nil
}

// GenerateResponse generates a response using Anthropic's Claude API
func (a *AnthropicProvider) GenerateResponse(ctx context.Context, prompt string) (string, error) {
	// Add user prompt
	messages := []Message{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	// Get system prompt if configured
	systemPrompt := a.settings.GetSystemPrompt()

	request := AnthropicRequest{
		Model:     a.model,
		MaxTokens: a.settings.GetMaxTokens(),
		Messages:  messages,
		System:    systemPrompt, // Use system parameter instead of system message
	}

	return a.makeRequest(ctx, request)
}

// GenerateToolCall uses Claude to determine which tool to call
func (a *AnthropicProvider) GenerateToolCall(ctx context.Context, query string, tools []mcp.Tool) (*ToolCall, error) {
	// Format tools for the prompt
	toolsDesc := make([]string, len(tools))
	for i, tool := range tools {
		toolsDesc[i] = fmt.Sprintf("- %s: %s", tool.Name, tool.Description)
	}
	toolsText := strings.Join(toolsDesc, "\n")

	prompt := fmt.Sprintf(`You are an intelligent agent that selects appropriate tools to answer user queries.

User Query: %s

Available Tools:
%s

Please analyze the query and determine if any tools should be called. If a tool should be called, respond with a JSON object in this exact format:
{
  "tool_name": "name_of_tool",
  "arguments": {"key": "value"},
  "reasoning": "explanation of why this tool was chosen"
}

If no tools are needed, respond with: {"no_tool_needed": true}

For log-related queries, use these LogQL patterns:
- Error logs: {level="error"}
- Warning logs: {level="warn"}
- All logs: {job=~".+"}
- Specific service: {service="myservice"}

Response:`, query, toolsText)

	response, err := a.GenerateResponse(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool selection from Claude: %w", err)
	}

	// Parse the JSON response
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		// If JSON parsing fails, try to extract JSON from the response
		start := strings.Index(response, "{")
		end := strings.LastIndex(response, "}") + 1
		if start >= 0 && end > start {
			jsonStr := response[start:end]
			if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
				return nil, fmt.Errorf("failed to parse tool selection response: %w", err)
			}
		} else {
			return nil, fmt.Errorf("no valid JSON found in response: %s", response)
		}
	}

	// Check if no tool is needed
	if noTool, exists := result["no_tool_needed"]; exists && noTool == true {
		return nil, nil
	}

	// Extract tool call information
	toolName, ok := result["tool_name"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid tool_name in response")
	}

	reasoning, _ := result["reasoning"].(string)
	arguments, _ := result["arguments"].(map[string]interface{})
	if arguments == nil {
		arguments = make(map[string]interface{})
	}

	return &ToolCall{
		ToolName:  toolName,
		Arguments: arguments,
		Reasoning: reasoning,
	}, nil
}

// GenerateStructuredResults generates structured data from tool results
// Uses intelligent sampling and local processing to avoid sending large datasets to LLM
func (a *AnthropicProvider) GenerateStructuredResults(ctx context.Context, query string, toolResults []ToolResult) (*StructuredQueryResult, error) {
	const (
		maxResultLength = 2000 // Max characters per result to send to LLM
		maxSampleLines  = 10   // Max lines to sample from large results
	)

	// First, try to structure the data locally without LLM
	if localResult := a.tryLocalStructuring(query, toolResults); localResult != nil {
		return localResult, nil
	}

	// If local structuring fails, prepare summarized results for LLM
	toolSummaries := make([]string, 0, len(toolResults))

	for _, result := range toolResults {
		if !result.Success {
			toolSummaries = append(toolSummaries, fmt.Sprintf("Tool: %s\nError: %s", result.ToolName, result.Error))
			continue
		}

		dataStr, ok := result.Data.(string)
		if !ok {
			toolSummaries = append(toolSummaries, fmt.Sprintf("Tool: %s\nResult: [Non-string data]", result.ToolName))
			continue
		}

		// If result is small, include it fully
		if len(dataStr) <= maxResultLength {
			toolSummaries = append(toolSummaries, fmt.Sprintf("Tool: %s\nResult: %s", result.ToolName, dataStr))
			continue
		}

		// For large results, create a summary
		summary := a.summarizeResult(dataStr, maxSampleLines)
		toolSummaries = append(toolSummaries, fmt.Sprintf("Tool: %s\nResult Summary (truncated from %d chars):\n%s",
			result.ToolName, len(dataStr), summary))
	}

	toolResultsStr := strings.Join(toolSummaries, "\n\n")

	prompt := fmt.Sprintf(`You are a data analyst. The user asked: "%s"

Tool execution results (may be summarized for large datasets):
%s

Please analyze the results and return a JSON response with structured data that can be easily visualized in Grafana.

Your response MUST be a valid JSON object with this exact structure:
{
  "data": [
    {"column1": "value1", "column2": "value2"},
    {"column1": "value3", "column2": "value4"}
  ],
  "columns": ["column1", "column2"],
  "summary": "Brief summary of the findings",
  "success": true,
  "error_msg": ""
}

For log queries, structure the data with columns like: timestamp, level, message, service, etc.
For metrics, use appropriate column names and ensure numeric data is properly typed.
If there are errors, set success to false and provide error_msg.

Note: Some results may be truncated due to size limits. Focus on the structure and patterns shown.

JSON Response:`, query, toolResultsStr)

	response, err := a.GenerateResponse(ctx, prompt)
	if err != nil {
		return &StructuredQueryResult{
			Query:    query,
			Success:  false,
			ErrorMsg: fmt.Sprintf("Failed to generate structured results: %v", err),
		}, nil
	}

	// Parse the JSON response
	var result StructuredQueryResult

	start := strings.Index(response, "```json") + 7
	end := strings.LastIndex(response, "}") + 1

	response = response[start:end]
	// Try to extract JSON from response if it's wrapped in markdown or other text
	start = strings.Index(response, "{")
	end = strings.LastIndex(response, "}") + 1
	if start >= 0 && end > start {
		jsonStr := response[start:end]
		if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
			return &StructuredQueryResult{
				Query:    query,
				Success:  false,
				ErrorMsg: fmt.Sprintf("Failed to parse structured response: %v", err),
			}, nil
		}
	} else {
		return &StructuredQueryResult{
			Query:    query,
			Success:  false,
			ErrorMsg: "No valid JSON found in LLM response",
		}, nil
	}

	// Set query and add metadata
	result.Query = query
	if result.Metadata == nil {
		result.Metadata = make(map[string]interface{})
	}
	result.Metadata["tool_count"] = len(toolResults)
	result.Metadata["processed_at"] = time.Now()
	result.Metadata["arguments"] = toolResults[0].Arguments
	result.Metadata["tool_name"] = toolResults[0].ToolName

	return &result, nil
}

// tryLocalStructuring attempts to structure data locally without using LLM
// Returns nil if local structuring isn't possible
func (a *AnthropicProvider) tryLocalStructuring(query string, toolResults []ToolResult) *StructuredQueryResult {
	// Only attempt local structuring for single successful results
	if len(toolResults) != 1 || !toolResults[0].Success {
		return nil
	}

	result := toolResults[0]
	dataStr, ok := result.Data.(string)
	if !ok {
		return nil
	}

	// Try to detect and parse JSON data
	if strings.HasPrefix(strings.TrimSpace(dataStr), "{") || strings.HasPrefix(strings.TrimSpace(dataStr), "[") {
		if structured := a.tryParseJSONResult(query, result.ToolName, dataStr); structured != nil {
			return structured
		}
	}

	// Try to detect structured log data (common patterns)
	if a.looksLikeLogData(dataStr) {
		if structured := a.tryParseLogResult(query, result.ToolName, dataStr); structured != nil {
			return structured
		}
	}

	if strings.Contains(dataStr, "No logs found") {
		return &StructuredQueryResult{
			Query:   query,
			Data:    []map[string]interface{}{},
			Columns: []string{},
			Summary: "No logs found",
			Success: true,
			Metadata: map[string]interface{}{
				"tool_count":    1,
				"processed_at":  time.Now(),
				"local_parsing": true,
			},
		}
	}

	// If data is small enough and looks simple, we can let LLM handle it
	if len(dataStr) <= 1000 && strings.Count(dataStr, "\n") <= 20 {
		return nil // Let LLM handle small, simple data
	}

	// For large unstructured data, create a basic structure
	return &StructuredQueryResult{
		Query:   query,
		Data:    []map[string]interface{}{{"result": dataStr[:min(500, len(dataStr))]}},
		Columns: []string{"result"},
		Summary: fmt.Sprintf("Raw result from %s (truncated)", result.ToolName),
		Success: true,
		Metadata: map[string]interface{}{
			"tool_count":    1,
			"processed_at":  time.Now(),
			"local_parsing": true,
			"truncated":     len(dataStr) > 500,
		},
	}
}

// tryParseJSONResult attempts to parse JSON data into structured format
func (a *AnthropicProvider) tryParseJSONResult(query, toolName, dataStr string) *StructuredQueryResult {
	var jsonData interface{}
	if err := json.Unmarshal([]byte(dataStr), &jsonData); err != nil {
		return nil
	}

	// Handle array of objects (most common case)
	if dataArray, ok := jsonData.([]interface{}); ok {
		if len(dataArray) == 0 {
			return &StructuredQueryResult{
				Query:   query,
				Data:    []map[string]interface{}{},
				Columns: []string{},
				Summary: "No data returned",
				Success: true,
				Metadata: map[string]interface{}{
					"tool_count":    1,
					"processed_at":  time.Now(),
					"local_parsing": true,
				},
			}
		}

		// Extract columns from first object
		firstObj, ok := dataArray[0].(map[string]interface{})
		if !ok {
			return nil
		}

		columns := make([]string, 0, len(firstObj))
		for key := range firstObj {
			columns = append(columns, key)
		}

		// Convert to our format
		data := make([]map[string]interface{}, len(dataArray))
		for i, item := range dataArray {
			if obj, ok := item.(map[string]interface{}); ok {
				data[i] = obj
			}
		}

		return &StructuredQueryResult{
			Query:   query,
			Data:    data,
			Columns: columns,
			Summary: fmt.Sprintf("Parsed %d JSON records from %s", len(data), toolName),
			Success: true,
			Metadata: map[string]interface{}{
				"tool_count":    1,
				"processed_at":  time.Now(),
				"local_parsing": true,
			},
		}
	}

	// Handle single object
	if dataObj, ok := jsonData.(map[string]interface{}); ok {
		columns := make([]string, 0, len(dataObj))
		for key := range dataObj {
			columns = append(columns, key)
		}

		return &StructuredQueryResult{
			Query:   query,
			Data:    []map[string]interface{}{dataObj},
			Columns: columns,
			Summary: fmt.Sprintf("Parsed JSON object from %s", toolName),
			Success: true,
			Metadata: map[string]interface{}{
				"tool_count":    1,
				"processed_at":  time.Now(),
				"local_parsing": true,
			},
		}
	}

	return nil
}

// looksLikeLogData checks if data appears to be structured log format
func (a *AnthropicProvider) looksLikeLogData(dataStr string) bool {
	lines := strings.Split(dataStr, "\n")
	if len(lines) < 2 {
		return false
	}

	// Look for common log patterns
	logPatterns := []string{
		"timestamp", "time", "level", "message", "msg",
		"service", "logger", "@timestamp", "ts",
	}

	firstLine := strings.ToLower(lines[0])
	matchCount := 0
	for _, pattern := range logPatterns {
		if strings.Contains(firstLine, pattern) {
			matchCount++
		}
	}

	return matchCount >= 2 // At least 2 log-like fields
}

// tryParseLogResult attempts to parse log data into structured format
func (a *AnthropicProvider) tryParseLogResult(query, toolName, dataStr string) *StructuredQueryResult {
	lines := strings.Split(strings.TrimSpace(dataStr), "\n")
	if len(lines) < 2 {
		return nil
	}

	// Try to parse as JSON logs
	data := make([]map[string]interface{}, 0, len(lines))
	var columns []string

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			// continue // Skip non-JSON lines
		}

		if logEntry == nil {
			// probably raw log line
			// timestamp, {lables}, message
			// 2023-12-07T10:30:45Z {job=myapp,level=info} This is a log message

			// split line into timestamp, labels, message
			parts := strings.Split(line, " ")
			timestamp := parts[0]
			labels := parts[1]
			message := strings.Join(parts[2:], " ")

			logEntry = map[string]interface{}{
				"timestamp": timestamp,
				"labels":    labels,
				"message":   message,
			}
		}

		// Extract columns from first entry
		if len(columns) == 0 {
			for key := range logEntry {
				columns = append(columns, key)
			}
		}

		data = append(data, logEntry)

		// Limit processing for large datasets
		if len(data) >= 1000 {
			break
		}
	}

	if len(data) > 0 {
		return &StructuredQueryResult{
			Query:   query,
			Data:    data,
			Columns: columns,
			Summary: fmt.Sprintf("Parsed %d log entries from %s", len(data), toolName),
			Success: true,
			Metadata: map[string]interface{}{
				"tool_count":    1,
				"processed_at":  time.Now(),
				"local_parsing": true,
				"total_lines":   len(lines),
			},
		}
	}

	return nil
}

// summarizeResult creates a summary of large result data
func (a *AnthropicProvider) summarizeResult(dataStr string, maxLines int) string {
	lines := strings.Split(dataStr, "\n")

	if len(lines) <= maxLines {
		return dataStr
	}

	// Take first few lines and last few lines
	takeFirst := maxLines / 2
	takeLast := maxLines - takeFirst

	summary := strings.Builder{}

	// First lines
	for i := 0; i < takeFirst && i < len(lines); i++ {
		summary.WriteString(lines[i])
		summary.WriteString("\n")
	}

	// Truncation indicator
	summary.WriteString(fmt.Sprintf("... [%d lines omitted] ...\n", len(lines)-maxLines))

	// Last lines
	startLast := len(lines) - takeLast
	if startLast < takeFirst {
		startLast = takeFirst
	}

	for i := startLast; i < len(lines); i++ {
		summary.WriteString(lines[i])
		summary.WriteString("\n")
	}

	return summary.String()
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// FixQuerySyntax generates a corrected tool call based on syntax error feedback
func (a *AnthropicProvider) FixQuerySyntax(ctx context.Context, originalQuery string, toolName string, errorMessage string, tools []mcp.Tool) (*ToolCall, error) {
	// Format tools for the prompt
	toolsDesc := make([]string, len(tools))
	for i, tool := range tools {
		toolsDesc[i] = fmt.Sprintf("- %s: %s", tool.Name, tool.Description)
	}
	toolsText := strings.Join(toolsDesc, "\n")

	prompt := fmt.Sprintf(`You are an intelligent agent that fixes query syntax errors.

Original User Query: %s
Tool Used: %s
Error Message: %s

Available Tools:
%s

The previous query failed with a syntax error. Please analyze the error and generate a corrected tool call.

For LogQL queries, common syntax errors include:
- Missing quotes around label values
- Incorrect time range syntax (use [1h], [5m], etc.)
- Invalid label selectors
- Missing braces around selectors

Please respond with a JSON object in this exact format:
{
  "tool_name": "%s",
  "arguments": {"key": "value"},
  "reasoning": "explanation of the fix applied"
}

Corrected JSON Response:`, originalQuery, toolName, errorMessage, toolsText, toolName)

	response, err := a.GenerateResponse(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to get syntax fix from Claude: %w", err)
	}

	// Parse the JSON response
	var result map[string]interface{}

	// Try to extract JSON from response if it's wrapped in markdown or other text
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}") + 1
	if start >= 0 && end > start {
		jsonStr := response[start:end]
		if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
			return nil, fmt.Errorf("failed to parse syntax fix response: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no valid JSON found in syntax fix response: %s", response)
	}

	// Extract tool call information
	fixedToolName, ok := result["tool_name"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid tool_name in syntax fix response")
	}

	reasoning, _ := result["reasoning"].(string)
	arguments, _ := result["arguments"].(map[string]interface{})
	if arguments == nil {
		arguments = make(map[string]interface{})
	}

	return &ToolCall{
		ToolName:  fixedToolName,
		Arguments: arguments,
		Reasoning: reasoning,
	}, nil
}

// makeRequest makes an HTTP request to the Anthropic API
func (a *AnthropicProvider) makeRequest(ctx context.Context, request AnthropicRequest) (string, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", a.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response AnthropicResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(response.Content) == 0 {
		return "", fmt.Errorf("no content in response")
	}

	return response.Content[0].Text, nil
}
