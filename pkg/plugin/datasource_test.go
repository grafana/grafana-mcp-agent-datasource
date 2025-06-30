package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"grafana-mcpclient-datasource/pkg/models"
)

func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		timeout   time.Duration
		wantError bool
	}{
		{
			name:      "localhost SSE connection",
			serverURL: "http://localhost:8081/sse",
			timeout:   30 * time.Second,
			wantError: false,
		},
		{
			name:      "docker internal SSE connection",
			serverURL: "http://loki-mcp-server:8080/sse",
			timeout:   30 * time.Second,
			wantError: false,
		},
		{
			name:      "invalid URL",
			serverURL: "http://invalid-server:9999/sse",
			timeout:   5 * time.Second,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test settings
			config := models.MCPDataSourceSettings{
				ServerURL:         tt.serverURL,
				ConnectionTimeout: int(tt.timeout.Seconds()),
			}

			settingsJSON, err := json.Marshal(config)
			require.NoError(t, err)

			settings := backend.DataSourceInstanceSettings{
				JSONData: settingsJSON,
			}

			// Create datasource instance
			ctx, cancel := context.WithTimeout(context.Background(), tt.timeout+10*time.Second)
			defer cancel()

			instance, err := NewDatasource(ctx, settings)
			if tt.wantError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, instance)

			ds, ok := instance.(*Datasource)
			require.True(t, ok)
			require.NotNil(t, ds.mcpClient)

			// Test health check
			healthCtx, healthCancel := context.WithTimeout(context.Background(), tt.timeout)
			defer healthCancel()

			result, err := ds.CheckHealth(healthCtx, &backend.CheckHealthRequest{})
			require.NoError(t, err)
			assert.NotNil(t, result)

			if !tt.wantError {
				assert.Equal(t, backend.HealthStatusOk, result.Status)
				t.Logf("Health check result: %s", result.Message)
			}

			// Clean up
			ds.Dispose()
		})
	}
}

func TestListTools(t *testing.T) {
	// Test tool listing functionality specifically
	config := models.MCPDataSourceSettings{
		ServerURL:         "http://localhost:8081/sse",
		ConnectionTimeout: 30,
	}

	settingsJSON, err := json.Marshal(config)
	require.NoError(t, err)

	settings := backend.DataSourceInstanceSettings{
		JSONData: settingsJSON,
	}

	ctx := context.Background()
	instance, err := NewDatasource(ctx, settings)
	require.NoError(t, err)

	ds, ok := instance.(*Datasource)
	require.True(t, ok)
	defer ds.Dispose()

	// Test tool listing query
	queryJSON := `{"queryType": "list_tools"}`
	dataQuery := backend.DataQuery{
		JSON: json.RawMessage(queryJSON),
	}

	queryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response := ds.query(queryCtx, backend.PluginContext{}, dataQuery)

	if response.Error != nil {
		t.Logf("Query error: %v", response.Error)
	}

	assert.NoError(t, response.Error)
	assert.NotEmpty(t, response.Frames)

	if len(response.Frames) > 0 {
		frame := response.Frames[0]
		t.Logf("Found frame with %d fields", len(frame.Fields))
		for _, field := range frame.Fields {
			t.Logf("Field: %s, Length: %d", field.Name, field.Len())
		}
	}
}

func TestMCPClientDirect(t *testing.T) {
	// Test MCP client creation and basic operations directly
	config := models.MCPDataSourceSettings{
		ServerURL:         "http://localhost:8081/sse",
		ConnectionTimeout: 30,
	}

	t.Log("Creating MCP client...")
	mcpClient, err := createMCPClient(config)
	require.NoError(t, err)
	require.NotNil(t, mcpClient)
	defer mcpClient.Close()

	t.Log("Testing ListTools...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tools, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	require.NoError(t, err)
	require.NotNil(t, tools)

	t.Logf("Found %d tools", len(tools.Tools))
	for i, tool := range tools.Tools {
		t.Logf("Tool %d: %s - %s", i+1, tool.Name, tool.Description)
	}
}

func TestNaturalLanguageQuery(t *testing.T) {
	// Test the new natural language query functionality
	config := models.MCPDataSourceSettings{
		ServerURL:         "http://localhost:8080",
		Transport:         "sse", // Use SSE transport explicitly
		ConnectionTimeout: 30,
		LLMProvider:       "mock", // Use mock provider for testing
	}

	settingsJSON, err := json.Marshal(config)
	require.NoError(t, err)

	settings := backend.DataSourceInstanceSettings{
		JSONData: settingsJSON,
	}

	ctx := context.Background()
	instance, err := NewDatasource(ctx, settings)
	require.NoError(t, err)

	ds, ok := instance.(*Datasource)
	require.True(t, ok)
	defer ds.Dispose()

	testCases := []struct {
		name         string
		query        string
		expectFrames int
	}{
		{
			name:         "simple log query",
			query:        "Show me recent error logs",
			expectFrames: 3, // main result + tool calls + tool results
		},
		{
			name:         "general question",
			query:        "What tools are available?",
			expectFrames: 1, // just main result when no tools are called
		},
		{
			name:         "search query",
			query:        "Find logs containing 'database connection'",
			expectFrames: 3, // main result + tool calls + tool results
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create natural language query
			queryJSON := fmt.Sprintf(`{"queryType": "natural_language", "query": "%s"}`, tc.query)
			dataQuery := backend.DataQuery{
				JSON: json.RawMessage(queryJSON),
			}

			queryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			response := ds.query(queryCtx, backend.PluginContext{}, dataQuery)

			if response.Error != nil {
				t.Logf("Query error: %v", response.Error)
			}

			assert.NoError(t, response.Error)
			assert.NotEmpty(t, response.Frames)

			t.Logf("Query '%s' returned %d frames", tc.query, len(response.Frames))

			// Verify the main result frame
			if len(response.Frames) > 0 {
				mainFrame := response.Frames[0]
				assert.Equal(t, "natural_language_result", mainFrame.Name)
				assert.True(t, len(mainFrame.Fields) >= 4) // query, summary, tool_calls_count, processed_at

				// Check that we have the expected fields
				fieldNames := make([]string, len(mainFrame.Fields))
				for i, field := range mainFrame.Fields {
					fieldNames[i] = field.Name
				}
				assert.Contains(t, fieldNames, "query")
				assert.Contains(t, fieldNames, "summary")
				assert.Contains(t, fieldNames, "tool_calls_count")
				assert.Contains(t, fieldNames, "processed_at")

				t.Logf("Main frame fields: %v", fieldNames)

				// Check metadata
				assert.NotNil(t, mainFrame.Meta)
				assert.NotNil(t, mainFrame.Meta.Custom)
				if customMap, ok := mainFrame.Meta.Custom.(map[string]interface{}); ok {
					assert.Equal(t, "natural_language", customMap["queryType"])
				}
			}

			// Log information about all frames
			for i, frame := range response.Frames {
				t.Logf("Frame %d: %s with %d fields", i, frame.Name, len(frame.Fields))
				for j, field := range frame.Fields {
					t.Logf("  Field %d: %s (len=%d)", j, field.Name, field.Len())
				}
			}
		})
	}
}

func TestContainerNetworking(t *testing.T) {
	// Test to diagnose container networking issues
	urls := []string{
		"http://localhost:8081/sse",
		"http://127.0.0.1:8081/sse",
		"http://loki-mcp-server:8080/sse",
		"http://host.docker.internal:8081/sse",
	}

	for _, url := range urls {
		t.Run("test_"+url, func(t *testing.T) {
			config := models.MCPDataSourceSettings{
				ServerURL:         url,
				ConnectionTimeout: 10, // Shorter timeout for this test
			}

			t.Logf("Testing connection to: %s", url)

			start := time.Now()
			mcpClient, err := createMCPClient(config)
			duration := time.Since(start)

			t.Logf("Connection attempt took: %v", duration)

			if err != nil {
				t.Logf("Failed to connect to %s: %v", url, err)
			} else {
				t.Logf("Successfully connected to %s", url)
				mcpClient.Close()
			}
		})
	}
}
