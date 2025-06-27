package main

import (
	"context"
	"log"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Test SSE connection to Loki MCP server
	serverURL := "http://localhost:8081/sse"

	log.Printf("=== Debugging Session Management ===")
	log.Printf("Server URL: %s", serverURL)

	// Create SSE client
	log.Printf("1. Creating SSE client...")
	mcpClient, err := client.NewSSEMCPClient(serverURL)
	if err != nil {
		log.Fatalf("Failed to create SSE client: %v", err)
	}
	log.Printf("✓ SSE client created successfully")

	// Start the client
	log.Printf("2. Starting client...")
	startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startCancel()

	if err := mcpClient.Start(startCtx); err != nil {
		log.Fatalf("Failed to start client: %v", err)
	}
	log.Printf("✓ Client started successfully")

	// Initialize the client
	log.Printf("3. Initializing client...")
	initCtx, initCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer initCancel()

	initRequest := mcp.InitializeRequest{
		Request: mcp.Request{
			Method: "initialize",
		},
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "debug-session-client",
				Version: "1.0.0",
			},
		},
	}

	log.Printf("Sending initialize request with protocol version: %s", mcp.LATEST_PROTOCOL_VERSION)
	initResponse, err := mcpClient.Initialize(initCtx, initRequest)
	if err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}

	log.Printf("✓ Client initialized successfully")
	log.Printf("  - Protocol version: %s", initResponse.ProtocolVersion)
	log.Printf("  - Server name: %s", initResponse.ServerInfo.Name)
	log.Printf("  - Server version: %s", initResponse.ServerInfo.Version)
	if initResponse.Capabilities.Tools != nil {
		log.Printf("  - Tools capability: %+v", *initResponse.Capabilities.Tools)
	}

	// Wait a bit to ensure session is established
	log.Printf("4. Waiting 3 seconds for session to stabilize...")
	time.Sleep(3 * time.Second)

	// Try to list tools with multiple approaches
	log.Printf("5. Attempting to list tools...")

	// Approach 1: Simple request
	log.Printf("  Approach 1: Simple ListTools request")
	toolsCtx1, cancel1 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel1()

	tools, err := mcpClient.ListTools(toolsCtx1, mcp.ListToolsRequest{})
	if err != nil {
		log.Printf("  ❌ Approach 1 failed: %v", err)

		// Approach 2: Try with a fresh timeout
		log.Printf("  Approach 2: Fresh timeout context")
		time.Sleep(2 * time.Second)
		toolsCtx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel2()

		tools, err = mcpClient.ListTools(toolsCtx2, mcp.ListToolsRequest{})
		if err != nil {
			log.Printf("  ❌ Approach 2 failed: %v", err)

			// Approach 3: Try re-initializing
			log.Printf("  Approach 3: Re-initializing client")
			time.Sleep(2 * time.Second)

			initCtx2, initCancel2 := context.WithTimeout(context.Background(), 30*time.Second)
			defer initCancel2()

			_, err = mcpClient.Initialize(initCtx2, initRequest)
			if err != nil {
				log.Printf("  ❌ Re-initialization failed: %v", err)
			} else {
				log.Printf("  ✓ Re-initialization successful")

				// Try listing tools again
				toolsCtx3, cancel3 := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel3()

				tools, err = mcpClient.ListTools(toolsCtx3, mcp.ListToolsRequest{})
				if err != nil {
					log.Printf("  ❌ Approach 3 failed: %v", err)
					log.Printf("=== All approaches failed ===")
					mcpClient.Close()
					return
				}
			}
		}
	}

	log.Printf("✓ Successfully listed tools!")
	log.Printf("Found %d tools:", len(tools.Tools))
	for i, tool := range tools.Tools {
		log.Printf("  %d. %s: %s", i+1, tool.Name, tool.Description)
	}

	// Try to call a tool if available
	if len(tools.Tools) > 0 {
		log.Printf("6. Testing tool call...")
		toolToTest := tools.Tools[0]
		log.Printf("Testing tool: %s", toolToTest.Name)

		callCtx, callCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer callCancel()

		// Create a simple test call - assuming loki_query tool
		callRequest := mcp.CallToolRequest{
			Request: mcp.Request{
				Method: "tools/call",
			},
			Params: mcp.CallToolParams{
				Name: toolToTest.Name,
				Arguments: map[string]interface{}{
					"query": "{job=\"log-generator\"}", // Simple LogQL query
				},
			},
		}

		log.Printf("Calling tool with query: {job=\"log-generator\"}")
		result, err := mcpClient.CallTool(callCtx, callRequest)
		if err != nil {
			log.Printf("❌ Tool call failed: %v", err)
		} else {
			log.Printf("✓ Tool call succeeded!")
			log.Printf("Result: %+v", result)
		}
	}

	// Clean up
	mcpClient.Close()
	log.Printf("=== Debug session completed ===")
}
