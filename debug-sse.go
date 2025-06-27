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

	log.Printf("Creating SSE client for URL: %s", serverURL)

	// Create SSE client
	mcpClient, err := client.NewSSEMCPClient(serverURL)
	if err != nil {
		log.Fatalf("Failed to create SSE client: %v", err)
	}

	log.Printf("SSE client created successfully")

	// Start the client with timeout
	startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer startCancel()

	log.Printf("Starting client...")
	if err := mcpClient.Start(startCtx); err != nil {
		log.Fatalf("Failed to start client: %v", err)
	}

	log.Printf("Client started, initializing...")

	// Initialize the client
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
				Name:    "debug-sse-client",
				Version: "1.0.0",
			},
		},
	}

	log.Printf("Sending initialize request...")
	initResponse, err := mcpClient.Initialize(initCtx, initRequest)
	if err != nil {
		log.Fatalf("Failed to initialize client: %v", err)
	}

	log.Printf("Client initialized successfully. Response: %+v", initResponse)

	// Try to list tools
	log.Printf("Listing tools...")
	toolsCtx, toolsCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer toolsCancel()

	tools, err := mcpClient.ListTools(toolsCtx, mcp.ListToolsRequest{})
	if err != nil {
		log.Fatalf("Failed to list tools: %v", err)
	}

	log.Printf("Found %d tools:", len(tools.Tools))
	for _, tool := range tools.Tools {
		log.Printf("  - %s: %s", tool.Name, tool.Description)
	}

	// Clean up
	mcpClient.Close()
	log.Printf("Done!")
}
