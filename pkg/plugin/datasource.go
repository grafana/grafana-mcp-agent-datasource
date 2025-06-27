package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"grafana-mcpclient-datasource/pkg/models"
)

// Make sure Datasource implements required interfaces. This is important to do
// since otherwise we will only get a not implemented error response from plugin in
// runtime. In this example datasource instance implements backend.QueryDataHandler,
// backend.CheckHealthHandler, backend.StreamHandler interfaces. Plugin should not
// implement all these interfaces - only those which are required for a particular task.
var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// NewDatasource creates a new datasource instance.
func NewDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	log.DefaultLogger.Info("Creating new MCP datasource instance")

	var config models.MCPDataSourceSettings
	if err := json.Unmarshal(settings.JSONData, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	// Create MCP client
	mcpClient, err := createMCPClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	return &Datasource{
		settings:  config,
		mcpClient: mcpClient,
		logger:    log.DefaultLogger,
	}, nil
}

// Datasource is an example datasource which can respond to data queries, reports
// its health and has streaming skills.
type Datasource struct {
	settings  models.MCPDataSourceSettings
	mcpClient *client.Client
	logger    log.Logger
}

func createMCPClient(config models.MCPDataSourceSettings) (*client.Client, error) {
	if config.ServerURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}

	// Parse the server URL to determine transport type
	serverURL, err := url.Parse(config.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}

	var mcpClient *client.Client

	switch serverURL.Scheme {
	case "http", "https":
		// Use SSE transport for HTTP(S) URLs
		mcpClient, err = client.NewSSEMCPClient(config.ServerURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create SSE client: %w", err)
		}
	case "ws", "wss":
		// For WebSocket URLs, we could use streamable HTTP as an alternative
		// Convert ws:// to http:// and wss:// to https://
		httpURL := config.ServerURL
		if serverURL.Scheme == "ws" {
			httpURL = "http" + httpURL[2:]
		} else if serverURL.Scheme == "wss" {
			httpURL = "https" + httpURL[3:]
		}
		mcpClient, err = client.NewStreamableHttpClient(httpURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create streamable HTTP client: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported URL scheme: %s (supported: http, https, ws, wss)", serverURL.Scheme)
	}

	// Start the client connection
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.ConnectionTimeout)*time.Second)
	defer cancel()

	if err := mcpClient.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start MCP client: %w", err)
	}

	// Initialize the client
	initRequest := mcp.InitializeRequest{
		Request: mcp.Request{
			Method: "initialize",
		},
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{
				// Add any specific capabilities we need
			},
			ClientInfo: mcp.Implementation{
				Name:    "grafana-mcp-datasource",
				Version: "1.0.0",
			},
		},
	}

	_, err = mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		mcpClient.Close()
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	return mcpClient, nil
}

// Dispose here tells plugin SDK that plugin wants to clean up resources when a new instance
// created. As soon as datasource settings change detected by SDK old datasource instance will
// be disposed and a new one will be created using NewSampleDatasource factory function.
func (d *Datasource) Dispose() {
	// Clean up datasource instance resources.
	if d.mcpClient != nil {
		d.mcpClient.Close()
	}
}

// QueryData handles multiple queries and returns multiple responses.
// req contains the queries []DataQuery (where each query contains RefID as a unique identifier).
// The QueryDataResponse contains a map of RefID to the response for each query, and each response
// contains Frames ([]*Frame).
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	d.logger.Info("QueryData called", "queries", len(req.Queries))

	// Create response struct
	response := backend.NewQueryDataResponse()

	// Loop over queries and execute them individually.
	for _, q := range req.Queries {
		res := d.query(ctx, req.PluginContext, q)

		// Save the response in a hashmap
		// based on with RefID as identifier
		response.Responses[q.RefID] = res
	}

	return response, nil
}

func (d *Datasource) query(ctx context.Context, pCtx backend.PluginContext, query backend.DataQuery) backend.DataResponse {
	var response backend.DataResponse

	// Unmarshal the JSON into our query model.
	var qm models.MCPQuery

	if err := json.Unmarshal(query.JSON, &qm); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("json unmarshal: %v", err.Error()))
	}

	// Execute the query based on type
	switch qm.QueryType {
	case "natural_language":
		return d.executeNaturalLanguageQuery(ctx, qm)
	case "tool_call":
		return d.executeToolCall(ctx, qm)
	case "list_tools":
		return d.listTools(ctx)
	default:
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("unknown query type: %s", qm.QueryType))
	}

	return response
}

func (d *Datasource) executeNaturalLanguageQuery(ctx context.Context, query models.MCPQuery) backend.DataResponse {
	d.logger.Info("Executing natural language query", "query", query.Query)

	if query.Query == "" {
		return backend.ErrDataResponse(backend.StatusBadRequest, "query text is required for natural language queries")
	}

	// For natural language queries, we need to:
	// 1. Analyze the query to determine which tools to use
	// 2. Execute the appropriate tools
	// 3. Format the results

	// For now, let's implement a simple approach where we list available tools
	// and return information about them along with the query
	tools, err := d.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("failed to list tools: %v", err))
	}

	// Create a frame to hold the query results
	frame := data.NewFrame("natural_language_query")
	frame.Fields = append(frame.Fields,
		data.NewField("query", nil, []string{query.Query}),
		data.NewField("available_tools", nil, []int{len(tools.Tools)}),
		data.NewField("timestamp", nil, []time.Time{time.Now()}),
	)

	// Add tool information as metadata
	toolNames := make([]string, len(tools.Tools))
	for i, tool := range tools.Tools {
		toolNames[i] = tool.Name
	}
	frame.Meta = &data.FrameMeta{
		Custom: map[string]interface{}{
			"tools":     toolNames,
			"query":     query.Query,
			"queryType": "natural_language",
		},
	}

	return backend.DataResponse{
		Frames: []*data.Frame{frame},
	}
}

func (d *Datasource) executeToolCall(ctx context.Context, query models.MCPQuery) backend.DataResponse {
	d.logger.Info("Executing tool call", "tool", query.ToolName, "args", query.ToolArguments)

	if query.ToolName == "" {
		return backend.ErrDataResponse(backend.StatusBadRequest, "tool name is required for tool call queries")
	}

	// Prepare arguments
	var args interface{}
	if query.ToolArguments != "" {
		if err := json.Unmarshal([]byte(query.ToolArguments), &args); err != nil {
			return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("invalid tool arguments JSON: %v", err))
		}
	}

	// Execute the tool
	result, err := d.mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Request: mcp.Request{
			Method: "tools/call",
		},
		Params: mcp.CallToolParams{
			Name:      query.ToolName,
			Arguments: args,
		},
	})
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("tool execution failed: %v", err))
	}

	// Create frame with tool results
	frame := data.NewFrame("tool_call_result")
	frame.Fields = append(frame.Fields,
		data.NewField("tool_name", nil, []string{query.ToolName}),
		data.NewField("success", nil, []bool{!result.IsError}),
		data.NewField("timestamp", nil, []time.Time{time.Now()}),
	)

	// Add result content
	if len(result.Content) > 0 {
		resultTexts := make([]string, len(result.Content))
		for i, content := range result.Content {
			if textContent, ok := mcp.AsTextContent(content); ok {
				resultTexts[i] = textContent.Text
			} else {
				resultTexts[i] = fmt.Sprintf("Non-text content: %T", content)
			}
		}
		frame.Fields = append(frame.Fields,
			data.NewField("result", nil, resultTexts),
		)
	}

	frame.Meta = &data.FrameMeta{
		Custom: map[string]interface{}{
			"toolName":  query.ToolName,
			"toolArgs":  query.ToolArguments,
			"isError":   result.IsError,
			"queryType": "tool_call",
		},
	}

	return backend.DataResponse{
		Frames: []*data.Frame{frame},
	}
}

func (d *Datasource) listTools(ctx context.Context) backend.DataResponse {
	d.logger.Info("Listing available tools")

	tools, err := d.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("failed to list tools: %v", err))
	}

	// Create frame with tool information
	frame := data.NewFrame("tools")

	toolNames := make([]string, len(tools.Tools))
	toolDescriptions := make([]string, len(tools.Tools))

	for i, tool := range tools.Tools {
		toolNames[i] = tool.Name
		if tool.Description != "" {
			toolDescriptions[i] = tool.Description
		} else {
			toolDescriptions[i] = "No description available"
		}
	}

	frame.Fields = append(frame.Fields,
		data.NewField("name", nil, toolNames),
		data.NewField("description", nil, toolDescriptions),
	)

	frame.Meta = &data.FrameMeta{
		Custom: map[string]interface{}{
			"queryType": "list_tools",
			"toolCount": len(tools.Tools),
		},
	}

	return backend.DataResponse{
		Frames: []*data.Frame{frame},
	}
}

// CheckHealth handles health checks sent from Grafana to the plugin.
// The main use case for these health checks is the test button on the
// datasource configuration page which allows users to verify that
// a datasource is working as expected.
func (d *Datasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	d.logger.Info("CheckHealth called")

	if d.mcpClient == nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "MCP client not initialized",
		}, nil
	}

	// Test connection by trying to ping the server
	err := d.mcpClient.Ping(ctx)
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Failed to ping MCP server: %v", err),
		}, nil
	}

	// Try to list tools to verify functionality
	_, err = d.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Failed to list tools: %v", err),
		}, nil
	}

	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "MCP connection is healthy",
	}, nil
}

// CallResource handles resource calls sent from the frontend
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	d.logger.Info("CallResource called", "path", req.Path, "method", req.Method)

	switch req.Path {
	case "tools":
		return d.handleToolsResource(ctx, req, sender)
	case "servers":
		return d.handleServersResource(ctx, req, sender)
	default:
		return sender.Send(&backend.CallResourceResponse{
			Status: 404,
			Body:   []byte("Resource not found"),
		})
	}
}

func (d *Datasource) handleToolsResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	tools, err := d.mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: 500,
			Body:   []byte(fmt.Sprintf("Failed to list tools: %v", err)),
		})
	}

	response, err := json.Marshal(tools.Tools)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: 500,
			Body:   []byte(fmt.Sprintf("Failed to marshal tools: %v", err)),
		})
	}

	return sender.Send(&backend.CallResourceResponse{
		Status: 200,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body: response,
	})
}

func (d *Datasource) handleServersResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	// Return information about the connected MCP server
	serverInfo := map[string]interface{}{
		"serverUrl": d.settings.ServerURL,
		"transport": d.settings.Transport,
		"connected": d.mcpClient != nil,
	}

	response, err := json.Marshal(serverInfo)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: 500,
			Body:   []byte(fmt.Sprintf("Failed to marshal server info: %v", err)),
		})
	}

	return sender.Send(&backend.CallResourceResponse{
		Status: 200,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body: response,
	})
}
