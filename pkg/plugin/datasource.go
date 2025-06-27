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
	log.DefaultLogger.Info("Creating new MCP datasource instance",
		"uid", settings.UID,
		"id", settings.ID,
		"name", settings.Name,
		"url", settings.URL)

	var config models.MCPDataSourceSettings
	if err := json.Unmarshal(settings.JSONData, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	log.DefaultLogger.Info("Parsed MCP config",
		"serverURL", config.ServerURL,
		"transport", config.Transport,
		"timeout", config.ConnectionTimeout)

	return &Datasource{
		settings:       config,
		mcpClient:      nil, // Lazy initialization
		logger:         log.DefaultLogger,
		datasourceUID:  settings.UID,
		datasourceID:   settings.ID,
		datasourceName: settings.Name,
	}, nil
}

// Datasource is an example datasource which can respond to data queries, reports
// its health and has streaming skills.
type Datasource struct {
	settings       models.MCPDataSourceSettings
	mcpClient      *client.Client
	logger         log.Logger
	datasourceUID  string
	datasourceID   int64
	datasourceName string
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

	// Use a reasonable timeout for connection and initialization
	connectionTimeout := config.ConnectionTimeout
	if connectionTimeout <= 0 {
		connectionTimeout = 30 // Default to 30 seconds
	}

	log.DefaultLogger.Info("Starting MCP client", "url", config.ServerURL, "timeout", connectionTimeout)

	// For SSE clients, use a context that doesn't get cancelled to maintain the persistent connection
	// The SSE client needs a long-lived context for the stream to stay alive
	clientCtx := context.Background()

	if err := mcpClient.Start(clientCtx); err != nil {
		return nil, fmt.Errorf("failed to start MCP client: %w", err)
	}

	log.DefaultLogger.Info("Initializing MCP client")

	// Use the same persistent context for initialization
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "grafana-mcp-datasource",
		Version: "1.0.0",
	}
	initRequest.Params.Capabilities = mcp.ClientCapabilities{}

	log.DefaultLogger.Info("About to call mcpClient.Initialize()")

	initResult, err := mcpClient.Initialize(clientCtx, initRequest)
	if err != nil {
		log.DefaultLogger.Error("mcpClient.Initialize() failed", "error", err)
		mcpClient.Close()
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	log.DefaultLogger.Info("mcpClient.Initialize() succeeded", "result", initResult)

	// Test ListTools immediately after initialization to check if the issue is timing-related
	log.DefaultLogger.Info("Testing ListTools immediately after initialization")
	testCtx, testCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer testCancel()

	testTools, testErr := mcpClient.ListTools(testCtx, mcp.ListToolsRequest{})
	if testErr != nil {
		log.DefaultLogger.Error("Immediate ListTools test failed", "error", testErr)
	} else {
		log.DefaultLogger.Info("Immediate ListTools test succeeded", "toolCount", len(testTools.Tools))
	}

	log.DefaultLogger.Info("MCP client successfully created and initialized")

	return mcpClient, nil
}

// getMCPClient returns the MCP client, creating it if necessary (lazy initialization)
func (d *Datasource) getMCPClient() (*client.Client, error) {
	d.logger.Info("getMCPClient() called", "hasExistingClient", d.mcpClient != nil)

	if d.mcpClient != nil {
		d.logger.Debug("Reusing existing MCP client")
		return d.mcpClient, nil
	}

	d.logger.Info("Creating MCP client for datasource",
		"uid", d.datasourceUID,
		"serverURL", d.settings.ServerURL)

	d.logger.Info("About to call createMCPClient()")
	mcpClient, err := createMCPClient(d.settings)
	if err != nil {
		d.logger.Error("createMCPClient() failed", "error", err)
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	d.logger.Info("createMCPClient() succeeded, caching client")
	d.mcpClient = mcpClient
	d.logger.Info("MCP client created and cached", "uid", d.datasourceUID)
	return d.mcpClient, nil
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
	mcpClient, err := d.getMCPClient()
	if err != nil {
		d.logger.Error("Failed to get MCP client", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("failed to get MCP client: %v", err))
	}

	toolsCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tools, err := mcpClient.ListTools(toolsCtx, mcp.ListToolsRequest{})
	if err != nil {
		d.logger.Error("Failed to list tools for natural language query", "query", query.Query, "error", err)
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

	// Execute the tool with timeout using background context
	mcpClient, err := d.getMCPClient()
	if err != nil {
		d.logger.Error("Failed to get MCP client", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("failed to get MCP client: %v", err))
	}

	toolCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := mcpClient.CallTool(toolCtx, mcp.CallToolRequest{
		Request: mcp.Request{
			Method: "tools/call",
		},
		Params: mcp.CallToolParams{
			Name:      query.ToolName,
			Arguments: args,
		},
	})
	if err != nil {
		d.logger.Error("Tool execution failed", "tool", query.ToolName, "error", err)
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

	mcpClient, err := d.getMCPClient()
	if err != nil {
		d.logger.Error("Failed to get MCP client", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("failed to get MCP client: %v", err))
	}

	// Add timeout context for the operation using background context
	toolsCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tools, err := mcpClient.ListTools(toolsCtx, mcp.ListToolsRequest{})
	if err != nil {
		d.logger.Error("ListTools failed", "error", err)
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
	d.logger.Info("CheckHealth called", "uid", d.datasourceUID)

	mcpClient, err := d.getMCPClient()
	if err != nil {
		d.logger.Error("Failed to get MCP client", "error", err)
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Failed to initialize MCP client: %v", err),
		}, nil
	}

	// Create a fresh timeout context for health check operations
	// Use background context to avoid inheriting Grafana's shorter timeout
	healthCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Skip Ping() as it might not be supported by the Loki MCP server
	// Try to list tools directly to verify functionality
	d.logger.Info("Listing MCP tools to verify connection")
	tools, err := mcpClient.ListTools(healthCtx, mcp.ListToolsRequest{})
	if err != nil {
		d.logger.Error("ListTools failed", "error", err)
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Failed to list tools: %v", err),
		}, nil
	}

	d.logger.Info("Health check successful", "toolCount", len(tools.Tools))
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: fmt.Sprintf("MCP connection is healthy - %d tools available", len(tools.Tools)),
	}, nil
}

// CallResource handles resource calls sent from the frontend
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	d.logger.Info("CallResource called", "path", req.Path, "method", req.Method)

	switch req.Path {
	case "health":
		return d.handleHealthResource(ctx, req, sender)
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
	mcpClient, err := d.getMCPClient()
	if err != nil {
		d.logger.Error("Failed to get MCP client", "error", err)
		return sender.Send(&backend.CallResourceResponse{
			Status: 500,
			Body:   []byte(fmt.Sprintf("Failed to get MCP client: %v", err)),
		})
	}

	toolsCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tools, err := mcpClient.ListTools(toolsCtx, mcp.ListToolsRequest{})
	if err != nil {
		d.logger.Error("Failed to list tools in resource handler", "error", err)
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

func (d *Datasource) handleHealthResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	// Perform health check and return detailed status
	healthResult, err := d.CheckHealth(ctx, &backend.CheckHealthRequest{})
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: 500,
			Body:   []byte(fmt.Sprintf("Health check failed: %v", err)),
		})
	}

	// Convert to a response format expected by frontend
	response := map[string]interface{}{
		"status":  "OK",
		"message": healthResult.Message,
	}

	if healthResult.Status != backend.HealthStatusOk {
		response["status"] = "ERROR"
	}

	// Try to get additional info if healthy
	if healthResult.Status == backend.HealthStatusOk {
		if mcpClient, err := d.getMCPClient(); err == nil {
			// Get tools info with timeout
			toolsCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			if tools, err := mcpClient.ListTools(toolsCtx, mcp.ListToolsRequest{}); err == nil {
				response["toolCount"] = len(tools.Tools)
			} else {
				d.logger.Warn("Failed to get tool count in health resource", "error", err)
			}
		}
	}

	responseBody, err := json.Marshal(response)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: 500,
			Body:   []byte(fmt.Sprintf("Failed to marshal health response: %v", err)),
		})
	}

	return sender.Send(&backend.CallResourceResponse{
		Status: 200,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body: responseBody,
	})
}

func (d *Datasource) handleServersResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	// Return information about the connected MCP server
	connected := false
	if d.mcpClient != nil {
		connected = true
	}

	serverInfo := map[string]interface{}{
		"serverUrl": d.settings.ServerURL,
		"transport": d.settings.Transport,
		"connected": connected,
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
