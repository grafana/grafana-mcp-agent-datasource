package models

import (
	"time"
)

// MCPDataSourceSettings represents the configuration for the MCP datasource
type MCPDataSourceSettings struct {
	// Server connection settings
	ServerURL         string `json:"serverUrl"`
	Transport         string `json:"transport"`         // "stream", "sse"
	StreamPath        string `json:"streamPath"`        // path for stream transport (default: "/stream")
	ConnectionTimeout int    `json:"connectionTimeout"` // timeout in seconds

	// Authentication settings
	AuthType      string            `json:"authType"` // "none", "basic", "bearer", "oauth"
	Username      string            `json:"username"`
	Password      string            `json:"password"`
	BearerToken   string            `json:"bearerToken"`
	CustomHeaders map[string]string `json:"customHeaders"`

	// LLM settings for natural language processing
	LLMProvider string `json:"llmProvider"` // "openai", "anthropic", "azure"
	LLMModel    string `json:"llmModel"`    // model name (e.g., "gpt-4", "claude-3-sonnet")
	LLMAPIKey   string `json:"llmApiKey"`   // API key for LLM service

	// Advanced settings
	MaxRetries        int  `json:"maxRetries"`
	RetryInterval     int  `json:"retryInterval"` // interval in seconds
	EnableStreaming   bool `json:"enableStreaming"`
	EnableCompression bool `json:"enableCompression"`
	MaxMessageSize    int  `json:"maxMessageSize"`

	// Query settings
	DefaultQueryTimeout  int `json:"defaultQueryTimeout"` // timeout in seconds
	MaxConcurrentQueries int `json:"maxConcurrentQueries"`
}

// MCPQuery represents a query to be executed against an MCP server
type MCPQuery struct {
	// Query identification
	QueryType string `json:"queryType"` // "natural_language", "tool_call", "list_tools", "list_resources", "get_prompt"

	// Natural language query
	Query string `json:"query"`

	// Tool call query
	ToolName      string `json:"toolName"`
	ToolArguments string `json:"toolArguments"` // JSON string

	// Resource query
	ResourceURI string `json:"resourceUri"`

	// Prompt query
	PromptName      string            `json:"promptName"`
	PromptArguments map[string]string `json:"promptArguments"`

	// Advanced options
	Timeout       int                    `json:"timeout"`       // query timeout in seconds
	MaxResults    int                    `json:"maxResults"`    // maximum number of results to return
	UseCache      bool                   `json:"useCache"`      // whether to use response caching
	CustomOptions map[string]interface{} `json:"customOptions"` // additional query options
}

// MCPTool represents an MCP tool available on the server
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Schema      map[string]interface{} `json:"schema"`
}

// MCPResource represents an MCP resource available on the server
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// MCPPrompt represents an MCP prompt template available on the server
type MCPPrompt struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Arguments   []MCPPromptArgument `json:"arguments"`
}

// MCPPromptArgument represents an argument for an MCP prompt template
type MCPPromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Type        string `json:"type"`
}

// MCPServerCapabilities represents the capabilities supported by an MCP server
type MCPServerCapabilities struct {
	Tools     *MCPToolsCapability     `json:"tools,omitempty"`
	Resources *MCPResourcesCapability `json:"resources,omitempty"`
	Prompts   *MCPPromptsCapability   `json:"prompts,omitempty"`
	Logging   *MCPLoggingCapability   `json:"logging,omitempty"`
}

// MCPToolsCapability represents tool-related capabilities
type MCPToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// MCPResourcesCapability represents resource-related capabilities
type MCPResourcesCapability struct {
	Subscribe   bool `json:"subscribe"`
	ListChanged bool `json:"listChanged"`
}

// MCPPromptsCapability represents prompt-related capabilities
type MCPPromptsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// MCPLoggingCapability represents logging capabilities
type MCPLoggingCapability struct {
	// No specific fields defined in the spec yet
}

// MCPConnectionStatus represents the current connection status
type MCPConnectionStatus struct {
	Connected      bool                   `json:"connected"`
	ServerURL      string                 `json:"serverUrl"`
	LastConnected  *time.Time             `json:"lastConnected,omitempty"`
	LastError      string                 `json:"lastError,omitempty"`
	Capabilities   *MCPServerCapabilities `json:"capabilities,omitempty"`
	AvailableTools []MCPTool              `json:"availableTools,omitempty"`
}

// GetConnectionTimeout returns the connection timeout in seconds, with a default value
func (s *MCPDataSourceSettings) GetConnectionTimeout() time.Duration {
	if s.ConnectionTimeout <= 0 {
		return 30 * time.Second
	}
	return time.Duration(s.ConnectionTimeout) * time.Second
}

// GetMaxRetries returns the max retries with a default value
func (s *MCPDataSourceSettings) GetMaxRetries() int {
	if s.MaxRetries <= 0 {
		return 3
	}
	return s.MaxRetries
}

// GetRetryInterval returns the retry interval with a default value
func (s *MCPDataSourceSettings) GetRetryInterval() time.Duration {
	if s.RetryInterval <= 0 {
		return 5 * time.Second
	}
	return time.Duration(s.RetryInterval) * time.Second
}

// GetDefaultQueryTimeout returns the default query timeout with a default value
func (s *MCPDataSourceSettings) GetDefaultQueryTimeout() time.Duration {
	if s.DefaultQueryTimeout <= 0 {
		return 30 * time.Second
	}
	return time.Duration(s.DefaultQueryTimeout) * time.Second
}

// GetMaxConcurrentQueries returns the max concurrent queries with a default value
func (s *MCPDataSourceSettings) GetMaxConcurrentQueries() int {
	if s.MaxConcurrentQueries <= 0 {
		return 5
	}
	return s.MaxConcurrentQueries
}

// GetMaxMessageSize returns the max message size with a default value
func (s *MCPDataSourceSettings) GetMaxMessageSize() int {
	if s.MaxMessageSize <= 0 {
		return 1024 * 1024 // 1MB default
	}
	return s.MaxMessageSize
}
