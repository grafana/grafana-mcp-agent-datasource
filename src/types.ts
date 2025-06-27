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
}

export const DEFAULT_QUERY: Partial<MCPQuery> = {
  query: '',
  maxResults: 100,
  format: 'auto',
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
  serverUrl?: string;                   // MCP server URL
  transport?: 'websocket' | 'http';     // Transport protocol
  timeout?: number;                     // Timeout in seconds
  maxRetries?: number;                  // Maximum retry attempts
  retryInterval?: number;               // Retry interval in seconds
}

/**
 * Secure/encrypted configuration data
 */
export interface MCPSecureJsonData {
  apiKey?: string;
  authToken?: string;
  username?: string;
  password?: string;
  clientId?: string;
  clientSecret?: string;
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

/**
 * Test connection result
 */
export interface TestConnectionResult {
  success: boolean;
  message: string;
  serverInfo?: MCPServerInfo;
  capabilities?: MCPServerCapabilities;
  toolCount?: number;
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
