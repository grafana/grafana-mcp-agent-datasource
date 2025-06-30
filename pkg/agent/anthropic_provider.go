package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// AnthropicProvider implements LLM functionality using Anthropic's Claude API
type AnthropicProvider struct {
	apiKey  string
	model   string
	baseURL string
}

// AnthropicRequest represents the request structure for Claude API
type AnthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
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
func NewAnthropicProvider(apiKey, model string) (*AnthropicProvider, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("Anthropic API key is required")
	}

	if model == "" {
		model = "claude-3-5-sonnet-20241022" // Default model
	}

	return &AnthropicProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: "https://api.anthropic.com/v1/messages",
	}, nil
}

// GenerateResponse generates a response using Anthropic's Claude API
func (a *AnthropicProvider) GenerateResponse(ctx context.Context, prompt string) (string, error) {
	request := AnthropicRequest{
		Model:     a.model,
		MaxTokens: 1000,
		Messages: []Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
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
