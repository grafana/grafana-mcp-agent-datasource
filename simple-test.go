package main

import (
	"fmt"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	fmt.Println("Testing mcp-go imports...")

	// Just check if we can create a client
	_, err := client.NewSSEMCPClient("http://test")
	if err != nil {
		fmt.Printf("Client creation failed: %v\n", err)
	} else {
		fmt.Println("Client creation succeeded!")
	}

	// Test MCP types
	req := mcp.InitializeRequest{}
	fmt.Printf("Request type: %T\n", req)

	fmt.Println("Import test successful!")
}
