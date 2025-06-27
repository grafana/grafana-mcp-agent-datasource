import { 
  DataSourceInstanceSettings, 
  CoreApp, 
  ScopedVars, 
  DataQueryRequest,
  DataQueryResponse,
  TestDataSourceResponse,
  LoadingState
} from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv, getBackendSrv } from '@grafana/runtime';

import { 
  MCPQuery, 
  MCPDataSourceOptions, 
  DEFAULT_QUERY, 
  MCPTool, 
  MCPConnectionStatus,
  TestConnectionResult 
} from './types';

export class DataSource extends DataSourceWithBackend<MCPQuery, MCPDataSourceOptions> {
  url?: string;

  constructor(instanceSettings: DataSourceInstanceSettings<MCPDataSourceOptions>) {
    super(instanceSettings);
    this.url = instanceSettings.url;
  }

  getDefaultQuery(_: CoreApp): Partial<MCPQuery> {
    return DEFAULT_QUERY;
  }

  applyTemplateVariables(query: MCPQuery, scopedVars: ScopedVars) {
    return {
      ...query,
      query: getTemplateSrv().replace(query.query || '', scopedVars),
      toolName: query.toolName ? getTemplateSrv().replace(query.toolName, scopedVars) : undefined,
    };
  }

  filterQuery(query: MCPQuery): boolean {
    // if no query has been provided, prevent the query from being executed
    return !!(query.query && query.query.trim());
  }

  /**
   * Override query method to add custom MCP-specific logic
   */
  query(request: DataQueryRequest<MCPQuery>) {
    // Pre-process queries to ensure they have required fields
    const processedRequest = {
      ...request,
      targets: request.targets.map(target => ({
        ...target,
        query: target.query || '',
        maxResults: target.maxResults || 100,
        format: target.format || 'auto',
      })),
    };

    // Call the backend through the parent class
    return super.query(processedRequest);
  }

  /**
   * Test the datasource connection
   */
  async testDatasource(): Promise<TestDataSourceResponse> {
    try {
      const response = await this.getResource('health');
      return {
        status: 'success',
        message: response.message || 'Successfully connected to MCP server',
      };
    } catch (error: any) {
      return {
        status: 'error',
        message: error.message || 'Failed to connect to MCP server',
      };
    }
  }

  /**
   * Get available MCP tools from the server
   */
  async getAvailableTools(): Promise<MCPTool[]> {
    try {
      const response = await this.getResource('tools');
      return response.tools || [];
    } catch (error) {
      console.error('Failed to get available tools:', error);
      return [];
    }
  }

  /**
   * Get MCP server connection status
   */
  async getConnectionStatus(): Promise<MCPConnectionStatus> {
    try {
      const response = await this.getResource('status');
      return {
        connected: response.connected || false,
        serverInfo: response.serverInfo,
        capabilities: response.capabilities,
        tools: response.tools,
        lastConnected: response.lastConnected,
      };
    } catch (error: any) {
      return {
        connected: false,
        lastError: error.message,
      };
    }
  }

  /**
   * Test connection to MCP server with detailed results
   */
  async testConnection(): Promise<TestConnectionResult> {
    try {
      const response = await this.getResource('test-connection');
      return {
        success: true,
        message: response.message || 'Connection successful',
        serverInfo: response.serverInfo,
        capabilities: response.capabilities,
        toolCount: response.toolCount,
      };
    } catch (error: any) {
      return {
        success: false,
        message: error.message || 'Connection failed',
      };
    }
  }

  /**
   * Execute a specific MCP tool
   */
  async executeTool(toolName: string, args?: Record<string, any>): Promise<any> {
    try {
      const response = await this.postResource('execute-tool', {
        toolName,
        arguments: args || {},
      });
      return response;
    } catch (error) {
      console.error('Failed to execute tool:', error);
      throw error;
    }
  }

  /**
   * Get query suggestions based on available tools and server capabilities
   */
  async getQuerySuggestions(partialQuery?: string): Promise<string[]> {
    try {
      const response = await this.postResource('query-suggestions', {
        partialQuery: partialQuery || '',
      });
      return response.suggestions || [];
    } catch (error) {
      console.error('Failed to get query suggestions:', error);
      return [];
    }
  }

  /**
   * Validate a query before execution
   */
  async validateQuery(query: MCPQuery): Promise<{ isValid: boolean; errors: string[]; warnings: string[] }> {
    try {
      const response = await this.postResource('validate-query', query);
      return {
        isValid: response.isValid || false,
        errors: response.errors || [],
        warnings: response.warnings || [],
      };
    } catch (error) {
      console.error('Failed to validate query:', error);
      return {
        isValid: false,
        errors: ['Failed to validate query'],
        warnings: [],
      };
    }
  }

  /**
   * Get query history for this datasource
   */
  async getQueryHistory(limit: number = 50): Promise<MCPQuery[]> {
    try {
      const response = await this.getResource(`query-history?limit=${limit}`);
      return response.history || [];
    } catch (error) {
      console.error('Failed to get query history:', error);
      return [];
    }
  }

  /**
   * Save a query to history
   */
  async saveQueryToHistory(query: MCPQuery): Promise<void> {
    try {
      await this.postResource('query-history', query);
    } catch (error) {
      console.error('Failed to save query to history:', error);
    }
  }

  /**
   * Get server capabilities and information
   */
  async getServerInfo(): Promise<{ serverInfo?: any; capabilities?: any }> {
    try {
      const response = await this.getResource('server-info');
      return {
        serverInfo: response.serverInfo,
        capabilities: response.capabilities,
      };
    } catch (error) {
      console.error('Failed to get server info:', error);
      return {};
    }
  }

  /**
   * Refresh the connection to the MCP server
   */
  async refreshConnection(): Promise<boolean> {
    try {
      const response = await this.postResource('refresh-connection', {});
      return response.success || false;
    } catch (error) {
      console.error('Failed to refresh connection:', error);
      return false;
    }
  }

  /**
   * Helper method to handle backend resource requests with proper error handling
   */
  async getResource(path: string): Promise<any> {
    const url = `api/datasources/${this.id}/resources/${path}`;
    return getBackendSrv().get(url);
  }

  /**
   * Helper method to handle backend resource POST requests
   */
  async postResource(path: string, data: any): Promise<any> {
    const url = `api/datasources/${this.id}/resources/${path}`;
    return getBackendSrv().post(url, data);
  }
}
