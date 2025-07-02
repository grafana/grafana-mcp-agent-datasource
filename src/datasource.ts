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
import { map } from 'rxjs/operators';
import { Subject, Observable } from 'rxjs';

import { 
  MCPQuery, 
  MCPDataSourceOptions, 
  DEFAULT_QUERY, 
  MCPTool, 
  MCPConnectionStatus
} from './types';

export interface QueryUpdateEvent {
  refId: string;
  originalQuery: MCPQuery;
  generatedToolCall?: {
    toolName: string;
    arguments: Record<string, any>;
    originalQuery: string;
  };
}

export class DataSource extends DataSourceWithBackend<MCPQuery, MCPDataSourceOptions> {
  url?: string;
  private queryUpdatesSubject = new Subject<QueryUpdateEvent>();

  constructor(instanceSettings: DataSourceInstanceSettings<MCPDataSourceOptions>) {
    super(instanceSettings);
    this.url = instanceSettings.url;
  }

  /**
   * Observable that emits when queries are executed and tool calls are generated
   */
  get queryUpdates$(): Observable<QueryUpdateEvent> {
    return this.queryUpdatesSubject.asObservable();
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

    // Call the backend through the parent class and process the response
    return super.query(processedRequest).pipe(
      map((response: DataQueryResponse) => {
        // Process each data frame to extract generated tool calls from metadata
        if (response.data) {
          response.data.forEach((frame, index) => {
            const customMeta = frame.meta?.custom;
            if (customMeta && customMeta.generated_tool_call) {
              // Extract the generated tool call from the response metadata
              const generatedToolCall = customMeta.generated_tool_call;
              
              // Update the corresponding target with the generated tool call
              // This allows Grafana to persist it as part of the query configuration
              if (processedRequest.targets[index]) {
                const originalQuery = processedRequest.targets[index];
                const updatedGeneratedToolCall = {
                  toolName: generatedToolCall.toolName,
                  arguments: generatedToolCall.arguments,
                  originalQuery: generatedToolCall.originalQuery,
                };
                
                processedRequest.targets[index].generatedToolCall = updatedGeneratedToolCall;
                
                // Emit query update event for subscribers (like QueryEditor)
                this.queryUpdatesSubject.next({
                  refId: originalQuery.refId,
                  originalQuery: originalQuery,
                  generatedToolCall: updatedGeneratedToolCall,
                });
              }
            }
          });
        }
        
        return response;
      })
    );
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
  async getAvailableTools(forceRefresh = false): Promise<MCPTool[]> {
    try {
      const path = forceRefresh ? 'tools?refresh=true' : 'tools';
      const response = await this.getResource(path);
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

  /**
   * Clean up resources when datasource is destroyed
   */
  dispose() {
    this.queryUpdatesSubject.complete();
  }
}
