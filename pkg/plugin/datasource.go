package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"grafana-mcpclient-datasource/pkg/agent"
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

	// Read secure fields from DecryptedSecureJSONData
	if settings.DecryptedSecureJSONData != nil {
		// LLM API Key
		if llmApiKey, exists := settings.DecryptedSecureJSONData["llmApiKey"]; exists {
			config.LLMAPIKey = llmApiKey
		}

		// Handle secure arguments
		if config.Arguments == nil {
			config.Arguments = make(map[string]string)
		}

		// Load secure arguments from DecryptedSecureJSONData
		for _, argName := range config.SecureArguments {
			secureKey := fmt.Sprintf("arg_%s", argName)
			if argValue, exists := settings.DecryptedSecureJSONData[secureKey]; exists {
				config.Arguments[argName] = argValue
			}
		}
	}

	log.DefaultLogger.Info("Parsed MCP config",
		"serverURL", config.ServerURL,
		"transport", config.Transport,
		"streamPath", config.StreamPath,
		"timeout", config.ConnectionTimeout,
		"llmProvider", config.LLMProvider,
		"llmModel", config.LLMModel,
		"hasLLMApiKey", config.LLMAPIKey != "",
		"regularArgs", len(config.Arguments),
		"secureArgs", len(config.SecureArguments))

	// Log argument keys (not values for security)
	if len(config.Arguments) > 0 {
		argKeys := make([]string, 0, len(config.Arguments))
		for key := range config.Arguments {
			argKeys = append(argKeys, key)
		}
		log.DefaultLogger.Info("MCP arguments configured", "keys", argKeys)
	}

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

	// Determine transport type (default to stream since SSE is deprecated)
	transport := config.Transport
	if transport == "" {
		transport = "stream" // Default to stream
	}

	var mcpClient *client.Client

	switch serverURL.Scheme {
	case "http", "https":
		switch transport {
		case "sse":
			// SSE transport requires /sse endpoint
			sseURL := config.ServerURL
			if !strings.HasSuffix(sseURL, "/sse") {
				sseURL = strings.TrimSuffix(sseURL, "/") + "/sse"
			}
			mcpClient, err = client.NewSSEMCPClient(sseURL)
			if err != nil {
				return nil, fmt.Errorf("failed to create SSE client: %w", err)
			}
		case "stream":
			// Stream transport: use configured path (default: /stream)
			streamPath := config.StreamPath
			if streamPath == "" {
				streamPath = "/stream" // Default to /stream
			}

			// Ensure path starts with /
			if !strings.HasPrefix(streamPath, "/") {
				streamPath = "/" + streamPath
			}

			streamURL := strings.TrimSuffix(config.ServerURL, "/") + streamPath
			log.DefaultLogger.Info("Creating stream transport client", "url", streamURL, "path", streamPath)

			mcpClient, err = client.NewStreamableHttpClient(streamURL)
			if err != nil {
				return nil, fmt.Errorf("failed to create streamable HTTP client: %w", err)
			}
		default:
			return nil, fmt.Errorf("unsupported transport: %s (supported: stream, sse)", transport)
		}
	default:
		return nil, fmt.Errorf("unsupported URL scheme: %s (only HTTP and HTTPS are supported)", serverURL.Scheme)
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

	// Start the MCP client
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

	_, err = mcpClient.Initialize(clientCtx, initRequest)
	if err != nil {
		mcpClient.Close()
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	log.DefaultLogger.Info("MCP client successfully created and initialized")

	return mcpClient, nil
}

// getMCPClient returns the MCP client, creating it if necessary (lazy initialization)
func (d *Datasource) getMCPClient() (*client.Client, error) {
	if d.mcpClient != nil {
		return d.mcpClient, nil
	}

	mcpClient, err := createMCPClient(d.settings)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	d.mcpClient = mcpClient
	return d.mcpClient, nil
}

// getStoredToolsAsMCP converts stored tools to mcp.Tool format for the agent
func (d *Datasource) getStoredToolsAsMCP() []mcp.Tool {
	mcpTools := make([]mcp.Tool, len(d.settings.Tools))
	for i, tool := range d.settings.Tools {
		// Convert map[string]interface{} back to ToolInputSchema
		var inputSchema mcp.ToolInputSchema
		if tool.Schema != nil {
			schemaBytes, _ := json.Marshal(tool.Schema)
			json.Unmarshal(schemaBytes, &inputSchema)
		}

		mcpTools[i] = mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: inputSchema,
		}
	}
	if len(mcpTools) == 0 {
		// Fetch tools from server
		mcpClient, err := d.getMCPClient()
		if err != nil {
			d.logger.Error("Failed to get MCP client", "error", err)
			return nil
		}
		tools, err := mcpClient.ListTools(context.Background(), mcp.ListToolsRequest{})
		if err != nil {
			d.logger.Error("Failed to list tools", "error", err)
			return nil
		}
		mcpTools = tools.Tools
	}
	return mcpTools
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

func (d *Datasource) query(ctx context.Context, _ backend.PluginContext, query backend.DataQuery) backend.DataResponse {
	// Unmarshal the JSON into our query model.
	var qm models.MCPQuery

	if err := json.Unmarshal(query.JSON, &qm); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("json unmarshal: %v", err.Error()))
	}

	// Extract time range from Grafana request if user wants to use dashboard time range
	if qm.UseDashboardTimeRange {
		qm.TimeRangeFrom = query.TimeRange.From.Format("2006-01-02T15:04:05Z07:00")
		qm.TimeRangeTo = query.TimeRange.To.Format("2006-01-02T15:04:05Z07:00")
		d.logger.Info("Using dashboard time range", "from", qm.TimeRangeFrom, "to", qm.TimeRangeTo)
	}

	// Execute the query based on type
	switch qm.QueryType {
	case "natural_language":
		return d.executeQuery(ctx, qm)
	case "tool_call":
		return d.executeToolCall(ctx, qm)
	case "list_tools":
		return d.listTools(ctx)
	default:
		return d.executeQuery(ctx, qm)
	}
}

func (d *Datasource) executeQuery(ctx context.Context, query models.MCPQuery) backend.DataResponse {
	d.logger.Info("Executing natural language query", "query", query.Query)

	if query.Query == "" {
		return backend.ErrDataResponse(backend.StatusBadRequest, "query text is required for natural language queries")
	}

	// Get MCP client
	mcpClient, err := d.getMCPClient()
	if err != nil {
		d.logger.Error("Failed to get MCP client", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("failed to get MCP client: %v", err))
	}

	// Create agent for intelligent query processing
	agent, err := agent.NewAgent(mcpClient, d.settings)
	if err != nil {
		d.logger.Error("Failed to create agent", "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("failed to create agent: %v", err))
	}

	// Process the natural language query using the agent with structured results
	queryCtx, cancel := context.WithTimeout(ctx, 60*time.Second) // Longer timeout for LLM processing
	defer cancel()

	// Get stored tools to pass to the agent
	storedTools := d.getStoredToolsAsMCP()

	result, err := agent.ProcessQueryStructured(queryCtx, query.Query, query.ToolName, query.TimeRangeFrom, query.TimeRangeTo, query.GeneratedToolCall, storedTools)
	if err != nil {
		d.logger.Error("Failed to process natural language query", "query", query.Query, "error", err)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("failed to process query: %v", err))
	}

	if !result.Success {
		d.logger.Error("Failed to process natural language query", "query", query.Query, "error", result.ErrorMsg)
		return backend.ErrDataResponse(backend.StatusInternal, fmt.Sprintf("failed to process query: %v", result.ErrorMsg))
	}

	// Create a single data frame from the structured result
	frame := data.NewFrame("query_results")

	// Handle case where we have no data
	if len(result.Data) == 0 {
		// Create a frame with just the summary information
		// frame.Fields = append(frame.Fields,
		// 	data.NewField("message", nil, []string{result.Summary}),
		// 	data.NewField("query", nil, []string{result.Query}),
		// 	data.NewField("success", nil, []bool{result.Success}),
		// )

		frame.Meta = &data.FrameMeta{
			Custom: map[string]interface{}{
				"queryType":     "natural_language",
				"originalQuery": query.Query,
				"summary":       result.Summary,
			},
		}
		args := ""
		arguments, ok := result.Metadata["arguments"]
		if ok {
			args = fmt.Sprintf("%v", arguments)
		}
		frame.Meta.ExecutedQueryString = args
		frame.Meta.Notices = append(frame.Meta.Notices, data.Notice{
			Severity: data.NoticeSeverityInfo,
			Text:     result.Summary,
		})
	} else {
		// Create fields based on the structured data
		// Initialize field slices based on columns
		fieldData := make(map[string][]interface{})

		// Initialize all fields
		for _, col := range result.Columns {
			fieldData[col] = make([]interface{}, 0, len(result.Data))
		}

		// Populate field data
		for _, row := range result.Data {
			for _, col := range result.Columns {
				if val, exists := row[col]; exists {
					fieldData[col] = append(fieldData[col], val)
				} else {
					fieldData[col] = append(fieldData[col], nil)
				}
			}
		}

		// Create fields for the frame
		for _, col := range result.Columns {
			// Determine the field type based on the first non-nil value
			var field *data.Field
			values := fieldData[col]

			if len(values) > 0 {
				// Find first non-nil value to determine type
				var sampleValue interface{}
				for _, v := range values {
					if v != nil {
						sampleValue = v
						break
					}
				}

				switch sampleValue.(type) {
				case string:
					stringValues := make([]string, len(values))
					for i, v := range values {
						if v != nil {
							stringValues[i] = fmt.Sprintf("%v", v)
						}
					}
					field = data.NewField(col, nil, stringValues)
				case bool:
					boolValues := make([]*bool, len(values))
					for i, v := range values {
						if v != nil {
							if b, ok := v.(bool); ok {
								boolValues[i] = &b
							}
						}
					}
					field = data.NewField(col, nil, boolValues)
				case int, int32, int64:
					floatValues := make([]*float64, len(values))
					for i, v := range values {
						if v != nil {
							if val, ok := convertToFloat64(v); ok {
								floatValues[i] = &val
							}
						}
					}
					field = data.NewField(col, nil, floatValues)
				case float32, float64:
					floatValues := make([]*float64, len(values))
					for i, v := range values {
						if v != nil {
							if val, ok := convertToFloat64(v); ok {
								floatValues[i] = &val
							}
						}
					}
					field = data.NewField(col, nil, floatValues)
				default:
					// Default to string
					stringValues := make([]string, len(values))
					for i, v := range values {
						if v != nil {
							stringValues[i] = fmt.Sprintf("%v", v)
						}
					}
					field = data.NewField(col, nil, stringValues)
				}
			} else {
				// Empty field, default to string
				field = data.NewField(col, nil, []string{})
			}

			frame.Fields = append(frame.Fields, field)
		}
	}

	if frame.Meta == nil {
		// Add comprehensive metadata
		frame.Meta = &data.FrameMeta{
			Custom: map[string]interface{}{
				"queryType":     "natural_language",
				"originalQuery": query.Query,
				"summary":       result.Summary,
				"success":       result.Success,
				"rowCount":      len(result.Data),
				"columnCount":   len(result.Columns),
			},
		}
	}

	// Add all metadata from the structured result
	if result.Metadata != nil {
		customMeta := frame.Meta.Custom.(map[string]interface{})
		for key, value := range result.Metadata {
			customMeta[key] = value
		}
	}

	// Add error information if present
	if result.ErrorMsg != "" {
		customMeta := frame.Meta.Custom.(map[string]interface{})
		customMeta["error"] = result.ErrorMsg
	}

	return backend.DataResponse{
		Frames: []*data.Frame{frame},
	}
}

// Helper function to convert various numeric types to float64
func convertToFloat64(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
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
	// Check if we have stored tools from configuration
	if len(d.settings.Tools) > 0 {
		d.logger.Info("Using stored tools for tools resource", "count", len(d.settings.Tools))

		// Convert stored tools to mcp.Tool format for response
		mcpTools := d.getStoredToolsAsMCP()

		// Create response with tools wrapped in an object to match frontend expectations
		toolsResponse := map[string]interface{}{
			"tools": mcpTools,
		}

		response, err := json.Marshal(toolsResponse)
		if err != nil {
			return sender.Send(&backend.CallResourceResponse{
				Status: 500,
				Body:   []byte(fmt.Sprintf("Failed to marshal stored tools: %v", err)),
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

	// If no stored tools, try to fetch from server (fallback for development/testing)
	d.logger.Warn("No stored tools available, fetching from server as fallback")

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

	// Create response with tools wrapped in an object to match frontend expectations
	toolsResponse := map[string]interface{}{
		"tools": tools.Tools,
	}

	response, err := json.Marshal(toolsResponse)
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
