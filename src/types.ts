import { DataSourceJsonData } from '@grafana/data';
import { DataQuery } from '@grafana/schema';

/**
 * MCP Query interface that extends the standard DataQuery
 */
export interface MCPQuery extends DataQuery {
  query?: string;                       // Natural language query
  toolName?: string;                    // Specific MCP tool to use (optional)
  arguments?: Record<string, any>;      // Additional arguments for the tool
  maxResults?: number;                  // Maximum number of results to return
  format?: string;                      // Output format preference
  useDashboardTimeRange?: boolean;      // Whether to include dashboard time range in queries
  
  // Generated tool call (stored to avoid LLM calls on dashboard refresh)
  generatedToolCall?: {
    toolName: string;
    arguments: Record<string, any>;
    originalQuery: string;              // Original query text that generated this tool call
  };
}

export const DEFAULT_QUERY: Partial<MCPQuery> = {
  query: '',
  maxResults: 100,
  format: 'auto',
  useDashboardTimeRange: true, // Default to using dashboard time range
};

/**
 * MCP Tool definition as returned by the server
 */
export interface MCPTool {
  name: string;
  description?: string;
  inputSchema?: any;
}

/**
 * MCP Server capabilities
 */
export interface MCPServerCapabilities {
  tools?: {
    listChanged?: boolean;
  };
  resources?: {
    subscribe?: boolean;
    listChanged?: boolean;
  };
  prompts?: {
    listChanged?: boolean;
  };
  logging?: {};
}

/**
 * MCP Server information
 */
export interface MCPServerInfo {
  name: string;
  version: string;
}

/**
 * Connection status for MCP server
 */
export interface MCPConnectionStatus {
  connected: boolean;
  serverInfo?: MCPServerInfo;
  capabilities?: MCPServerCapabilities;
  tools?: MCPTool[];
  lastError?: string;
  lastConnected?: string;
}

/**
 * DataSource configuration options stored in Grafana
 */
export interface MCPDataSourceOptions extends DataSourceJsonData {
  serverUrl?: string;                   // MCP server URL (HTTP/HTTPS)
  transport?: 'stream' | 'sse';         // Transport protocol (stream is recommended, sse is deprecated)
  streamPath?: string;                  // Path for stream transport (default: /stream)
  timeout?: number;                     // Timeout in seconds
  maxRetries?: number;                  // Maximum retry attempts
  retryInterval?: number;               // Retry interval in seconds
  
  // Arguments to pass to MCP server
  arguments?: Record<string, string>;   // Regular arguments (e.g., database name, host)
  secureArguments?: string[];           // Names of arguments that are stored securely
  
  // Agent Configuration
  llmProvider?: 'anthropic' | 'openai' | 'mock';  // LLM provider for natural language processing
  llmModel?: string;                    // LLM model name (e.g., claude-3-5-sonnet-20241022, gpt-4)
  systemPrompt?: string;                // System prompt always sent to LLM
  maxTokens?: number;                   // Maximum tokens for LLM responses
  agentRetries?: number;                // Number of retry attempts for agent calls
}

/**
 * Secure/encrypted configuration data
 */
export interface MCPSecureJsonData {
  // LLM API Keys
  llmApiKey?: string;                   // API key for LLM provider (Anthropic, OpenAI, etc.)
  
  // Dynamic secure arguments - these are stored with 'arg_' prefix
  // e.g., if user adds secure argument 'password', it's stored as 'arg_password'
  [key: string]: string | undefined;
}

/**
 * Argument configuration for the UI
 */
export interface MCPArgument {
  key: string;
  value: string;
  isSecure: boolean;
  isNew?: boolean;  // for UI state management
}

/**
 * Query template for common query patterns
 */
export interface MCPQueryTemplate {
  name: string;
  description: string;
  query: string;
  toolName?: string;
  arguments?: Record<string, any>;
  category?: string;
}

/**
 * Default query templates
 */
export const DEFAULT_QUERY_TEMPLATES: MCPQueryTemplate[] = [
  {
    name: 'Simple Question',
    description: 'Ask a simple question to the MCP server',
    query: 'What is the current status?',
    category: 'General',
  },
  {
    name: 'List Items',
    description: 'List available items or resources',
    query: 'List all available items',
    category: 'Discovery',
  },
  {
    name: 'Get Information',
    description: 'Get detailed information about something',
    query: 'Get information about [topic]',
    category: 'Information',
  },
  {
    name: 'Search Query',
    description: 'Search for specific content',
    query: 'Search for [search term]',
    category: 'Search',
  },
];

/**
 * Validation result for configuration
 */
export interface ValidationResult {
  isValid: boolean;
  errors: string[];
  warnings: string[];
}



// Legacy types (for backward compatibility)
export interface MyQuery extends DataQuery {
  queryText?: string;
  constant: number;
}

export interface DataPoint {
  Time: number;
  Value: number;
}

export interface DataSourceResponse {
  datapoints: DataPoint[];
}

export interface MyDataSourceOptions extends DataSourceJsonData {
  path?: string;
}

export interface MySecureJsonData {
  apiKey?: string;
}
