package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Custom HTTP transport that logs all requests and responses
type debugTransport struct {
	base http.RoundTripper
}

func (d *debugTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	fmt.Printf("\n=== HTTP REQUEST ===\n")
	if dump, err := httputil.DumpRequest(r, true); err == nil {
		fmt.Printf("%s\n", dump)
	}

	start := time.Now()
	resp, err := d.base.RoundTrip(r)
	duration := time.Since(start)

	fmt.Printf("\n=== HTTP RESPONSE (took %v) ===\n", duration)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
		return resp, err
	}

	if dump, err := httputil.DumpResponse(resp, false); err == nil {
		fmt.Printf("%s\n", dump)
	}

	return resp, err
}

func main() {
	fmt.Println("=== Detailed MCP SSE Debug ===")

	// Set up custom HTTP client with debug transport
	debugTransport := &debugTransport{base: http.DefaultTransport}
	httpClient := &http.Client{
		Transport: debugTransport,
		Timeout:   60 * time.Second,
	}

	// Create SSE client with custom HTTP client
	fmt.Println("Creating SSE MCP client with debug transport...")
	mcpClient, err := client.NewSSEMCPClient("http://loki-mcp-server:8080/sse")
	if err != nil {
		log.Fatalf("Failed to create SSE client: %v", err)
	}

	// Unfortunately, we can't easily inject our custom HTTP client into the mcp-go library
	// So let's create a simpler test that shows what's happening

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("\n=== Starting MCP client ===")
	startTime := time.Now()
	if err := mcpClient.Start(ctx); err != nil {
		log.Fatalf("Failed to start client: %v", err)
	}
	fmt.Printf("Start took: %v\n", time.Since(startTime))

	fmt.Println("\n=== Initializing MCP client ===")
	initTime := time.Now()
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "debug-client",
		Version: "1.0.0",
	}
	initRequest.Params.Capabilities = mcp.ClientCapabilities{}

	_, err = mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	fmt.Printf("Initialize took: %v\n", time.Since(initTime))

	// Now let's try ListTools with detailed timing
	fmt.Println("\n=== Calling ListTools ===")
	listTime := time.Now()

	// Create a context with our own timeout
	listCtx, listCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer listCancel()

	// Add some debugging around the actual call
	fmt.Printf("About to call ListTools at %v\n", time.Now())
	tools, err := mcpClient.ListTools(listCtx, mcp.ListToolsRequest{})
	fmt.Printf("ListTools returned at %v (took %v)\n", time.Now(), time.Since(listTime))

	if err != nil {
		fmt.Printf("ListTools failed: %v\n", err)

		// Check if it's a timeout error
		if listCtx.Err() == context.DeadlineExceeded {
			fmt.Println("This was a context deadline exceeded error")
		}

		// Let's try to understand what type of error this is
		fmt.Printf("Error type: %T\n", err)
		fmt.Printf("Error details: %+v\n", err)
	} else {
		fmt.Printf("Success! Found %d tools:\n", len(tools.Tools))
		for i, tool := range tools.Tools {
			fmt.Printf("  %d. %s: %s\n", i+1, tool.Name, tool.Description)
		}
	}

	// Let's also try to manually inspect what the SSE stream is sending
	fmt.Println("\n=== Manual SSE Connection Test ===")
	manualSSETest(httpClient)

	fmt.Println("\n=== Cleanup ===")
	mcpClient.Close()
}

func manualSSETest(httpClient *http.Client) {
	fmt.Println("Making manual request to SSE endpoint...")

	req, err := http.NewRequest("GET", "http://loki-mcp-server:8080/sse", nil)
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		return
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Printf("Failed to make request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Response status: %s\n", resp.Status)
	fmt.Printf("Response headers:\n")
	for k, v := range resp.Header {
		fmt.Printf("  %s: %s\n", k, v)
	}

	// Read a bit of the response
	buffer := make([]byte, 1024)
	n, err := resp.Body.Read(buffer)
	if err != nil && err.Error() != "EOF" {
		fmt.Printf("Failed to read response: %v\n", err)
		return
	}

	fmt.Printf("First %d bytes of response:\n%s\n", n, buffer[:n])
}
