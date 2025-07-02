import React, { ChangeEvent, useState, useEffect } from 'react';
import { 
  InlineField, 
  Input, 
  Stack, 
  TextArea, 
  Select, 
  Button, 
  Collapse,
  Alert,
  Badge,
  HorizontalGroup,
  VerticalGroup,
  Card,
  IconButton,
  Checkbox
} from '@grafana/ui';
import { QueryEditorProps, SelectableValue } from '@grafana/data';
import { DataSource } from '../datasource';
import { 
  MCPDataSourceOptions, 
  MCPQuery, 
  MCPTool,
} from '../types';

type Props = QueryEditorProps<DataSource, MCPQuery, MCPDataSourceOptions>;

export function QueryEditor({ query, onChange, onRunQuery, datasource }: Props) {
  const [availableTools, setAvailableTools] = useState<MCPTool[]>([]);
  const [isLoadingTools, setIsLoadingTools] = useState(false);
  const [toolsError, setToolsError] = useState<string | null>(null);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [recentlyUpdated, setRecentlyUpdated] = useState(false);

  // Initialize query with defaults if needed
  const currentQuery: MCPQuery = {
    ...query,
    query: query.query || '',
    maxResults: query.maxResults || 100,
    format: query.format || 'auto',
    useDashboardTimeRange: query.useDashboardTimeRange ?? true, // Default to true
  };

  // Load available tools on component mount and when datasource changes
  useEffect(() => {
    loadAvailableTools();
  }, [datasource]);

  // Subscribe to query updates from datasource
  useEffect(() => {
    const subscription = datasource.queryUpdates$.subscribe((updateEvent) => {
      // Only update if this is for the current query
      if (updateEvent.refId === query.refId && updateEvent.generatedToolCall) {
        const updatedQuery = {
          ...currentQuery,
          generatedToolCall: updateEvent.generatedToolCall,
          toolName: updateEvent.generatedToolCall.toolName, // Auto-select the tool that was used
        };
        
        onChange(updatedQuery);
        
        // Show temporary update indicator
        setRecentlyUpdated(true);
        const timer = setTimeout(() => setRecentlyUpdated(false), 3000); // Clear after 3 seconds
        
        return () => clearTimeout(timer);
      }
    });

    // Cleanup subscription on unmount
    return () => subscription.unsubscribe();
  }, [query.refId, currentQuery, onChange, datasource]);

  const loadAvailableTools = async (forceRefresh = false) => {
    setIsLoadingTools(true);
    setToolsError(null);
    try {
      // Call the datasource to get real available tools from MCP server
      const tools = await datasource.getAvailableTools(forceRefresh);
      setAvailableTools(tools);
      
      if (tools.length === 0) {
        setToolsError('No tools available from MCP server');
      }
    } catch (error) {
      console.error('Failed to load available tools:', error);
      const errorMessage = error instanceof Error ? error.message : 'Unknown error';
      setToolsError(`Failed to load tools: ${errorMessage}`);
      // Fallback to empty array on error
      setAvailableTools([]);
    } finally {
      setIsLoadingTools(false);
    }
  };

  // Handler for the refresh button click
  const handleRefreshTools = () => {
    loadAvailableTools(true); // Force refresh when user clicks the button
  };

  // Query text change handler
  const onQueryChange = (event: ChangeEvent<HTMLTextAreaElement>) => {
    const newQuery = event.target.value;
    
    // Clear cached tool call if query text has changed (to trigger new LLM generation)
    const updatedQuery = { 
      ...currentQuery, 
      query: newQuery
    };
    
    if (currentQuery.generatedToolCall && currentQuery.generatedToolCall.originalQuery !== newQuery) {
      updatedQuery.generatedToolCall = undefined;
    }
    
    onChange(updatedQuery);
  };

  // Tool selection change handler
  const onToolChange = (option: SelectableValue<string>) => {
    onChange({ 
      ...currentQuery, 
      toolName: option.value 
    });
  };

  // Max results change handler
  const onMaxResultsChange = (event: ChangeEvent<HTMLInputElement>) => {
    const value = parseInt(event.target.value, 10);
    onChange({ 
      ...currentQuery, 
      maxResults: isNaN(value) ? 100 : value 
    });
  };

  // Format change handler
  const onFormatChange = (option: SelectableValue<string>) => {
    onChange({ 
      ...currentQuery, 
      format: option.value 
    });
  };

  // Dashboard time range toggle handler
  const onUseDashboardTimeRangeChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({
      ...currentQuery,
      useDashboardTimeRange: event.target.checked,
    });
  };

  // Run query handler
  const handleRunQuery = () => {
    if (!currentQuery.query.trim()) {
      return;
    }
    onRunQuery();
  };

  // Clear query handler
  const onClearQuery = () => {
    onChange({
      ...currentQuery,
      query: '',
      toolName: undefined,
      arguments: undefined,
      generatedToolCall: undefined, // Clear cached tool call when clearing query
    });
  };

  // Tool options for select dropdown
  const toolOptions: SelectableValue[] = [
    { label: 'Auto-select tool', value: undefined, description: 'Let MCP client choose the best tool' },
    ...availableTools.map(tool => ({
      label: tool.name,
      value: tool.name,
      description: tool.description,
    })),
  ];

  // Format options
  const formatOptions: SelectableValue[] = [
    { label: 'Auto', value: 'auto', description: 'Auto-detect best format' },
    { label: 'Table', value: 'table', description: 'Tabular data format' },
    { label: 'Time Series', value: 'timeseries', description: 'Time series data format' },
    { label: 'Text', value: 'text', description: 'Plain text format' },
    { label: 'JSON', value: 'json', description: 'JSON data format' },
  ];

  return (
    <VerticalGroup spacing="md">
      {/* Main Query Section */}
      <Card>
          <Stack direction="column" gap={2}>
            <InlineField 
              label="Query" 
              labelWidth={12}
              tooltip="Enter your query in natural language"
              grow
            >
              <HorizontalGroup spacing="sm">
                <TextArea
                  id="query-editor-query"
                  onChange={onQueryChange}
                  value={currentQuery.query}
                  placeholder="Ask your question in natural language, e.g., 'Show me the latest sales data' or 'What are the top performing products?'"
                  rows={3}
                  style={{ minWidth: '400px', flex: 1 }}
                />
                <VerticalGroup spacing="xs">
                  <Button 
                    variant="primary" 
                    size="sm"
                    onClick={handleRunQuery}
                    disabled={!currentQuery.query.trim()}
                    icon="play"
                  >
                    Run Query
                  </Button>
                  <Button 
                    variant="secondary" 
                    size="sm"
                    onClick={onClearQuery}
                    icon="times"
                  >
                    Clear
                  </Button>
                </VerticalGroup>
              </HorizontalGroup>
            </InlineField>

            <HorizontalGroup spacing="md">
              <InlineField label="Tool" labelWidth={12} tooltip="Select a specific MCP tool or let the system auto-select">
                <HorizontalGroup spacing="xs">
                  <Select
                    options={toolOptions}
                    value={currentQuery.toolName}
                    onChange={onToolChange}
                    placeholder="Auto-select tool"
                    width={25}
                    isLoading={isLoadingTools}
                  />
                  <IconButton
                    name="sync"
                    size="md"
                    tooltip="Refresh available tools"
                    onClick={handleRefreshTools}
                    disabled={isLoadingTools}
                  />
                </HorizontalGroup>
              </InlineField>
              
              {/* Tools status indicator */}
              {toolsError ? (
                <Badge color="red" text={toolsError} />
              ) : availableTools.length > 0 ? (
                <Badge color="green" text={`${availableTools.length} tools available`} />
              ) : !isLoadingTools ? (
                <Badge color="orange" text="No tools loaded" />
              ) : null}

              {/* Recently updated indicator */}
              {recentlyUpdated && (
                <Badge color="purple" text="Tool call generated!" />
              )}

              <Button 
                variant="secondary" 
                size="sm"
                onClick={() => setShowAdvanced(!showAdvanced)}
                icon="cog"
              >
                Advanced
              </Button>
            </HorizontalGroup>
          </Stack>
      </Card>

      {/* Advanced Options */}
      {showAdvanced && (
        <Card>
          <HorizontalGroup spacing="md">
            <InlineField 
              label="Max Results" 
              labelWidth={16}
              tooltip="Maximum number of results to return"
            >
              <Input
                id="query-editor-max-results"
                type="number"
                onChange={onMaxResultsChange}
                value={currentQuery.maxResults}
                placeholder="100"
                width={15}
                min={1}
                max={10000}
              />
            </InlineField>

            <InlineField 
              label="Format" 
              labelWidth={16}
              tooltip="Preferred output format"
            >
              <Select
                options={formatOptions}
                value={currentQuery.format}
                onChange={onFormatChange}
                width={20}
              />
            </InlineField>
          </HorizontalGroup>

          <HorizontalGroup spacing="md">
            <InlineField 
              label="Use Dashboard Time" 
              labelWidth={24}
              tooltip="Include the dashboard's selected time range in queries when no time is specified"
            >
              <Checkbox
                value={currentQuery.useDashboardTimeRange}
                onChange={onUseDashboardTimeRangeChange}
              />
            </InlineField>
          </HorizontalGroup>

          {currentQuery.toolName && (
            <Alert title="Tool Selected" severity="info">
              <p>
                Using tool: <Badge text={currentQuery.toolName} color="blue" />
              </p>
              <p>
                {availableTools.find(t => t.name === currentQuery.toolName)?.description}
              </p>
            </Alert>
          )}

          {currentQuery.generatedToolCall && currentQuery.generatedToolCall.originalQuery === currentQuery.query && (
            <Alert title="Generated Tool Call" severity="success">
              <p>
                Generated tool call: <Badge text={currentQuery.generatedToolCall.toolName} color="green" />
              </p>
              <p>
                This query will execute faster on subsequent runs as it uses the previously generated tool call and arguments.
              </p>
              {currentQuery.generatedToolCall.arguments && Object.keys(currentQuery.generatedToolCall.arguments).length > 0 && (
                <details style={{ marginTop: '8px' }}>
                  <summary style={{ cursor: 'pointer', fontSize: '12px' }}>View generated arguments</summary>
                  <pre style={{ fontSize: '11px', marginTop: '4px', padding: '8px', backgroundColor: '#f5f5f5', borderRadius: '4px' }}>
                    {JSON.stringify(currentQuery.generatedToolCall.arguments, null, 2)}
                  </pre>
                </details>
              )}
            </Alert>
          )}
        </Card>
      )}

      {/* Server Status */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '8px 0' }}>
        <div style={{ fontSize: '12px', color: '#6c7680' }}>
          Available tools: <Badge text={availableTools.length.toString()} color="green" />
        </div>
        <Button 
          variant="secondary" 
          size="sm"
          onClick={handleRefreshTools}
          icon="sync"
          disabled={isLoadingTools}
        >
          {isLoadingTools ? 'Loading...' : 'Refresh Tools'}
        </Button>
      </div>
    </VerticalGroup>
  );
}
