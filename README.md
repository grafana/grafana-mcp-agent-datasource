| :warning: Info           |
|:----------------------------|
| This experiment has been archived, we merged this effort with [Grafana MCP](https://github.com/grafana/mcp-grafana), please use that. |

# Grafana MCP Agent DataSource Plugin

A Grafana data source plugin that connects to [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) servers and enables natural language querying of external tools and data sources.

## 🎉 Built with mcp-go Library

This plugin leverages the excellent [mark3labs/mcp-go library](https://github.com/mark3labs/mcp-go) (6.1k+ stars) for robust, production-ready MCP client functionality. This provides:

- ✅ Full MCP protocol compliance
- ✅ Multiple transport options (stdio, SSE, streamable HTTP, in-process)
- ✅ Comprehensive error handling and retry logic
- ✅ Active maintenance and community support

## What is MCP?

The Model Context Protocol (MCP) is an open standard that enables Large Language Models (LLMs) to securely connect with external data sources and tools. MCP servers can:

- **Expose data** through Resources (like databases, files, APIs)
- **Provide functionality** through Tools (like calculations, external service calls)
- **Define interaction patterns** through Prompts (reusable templates)

## Features

- 🔗 **Universal MCP Connectivity**: Connect to any MCP-compliant server
- 🗣️ **Natural Language Queries**: Query your data sources using plain English
- 🛠️ **Tool Integration**: Execute MCP tools directly from Grafana
- 📊 **Data Visualization**: Convert MCP responses into Grafana data frames
- 🔧 **Multiple Transports**: Support for WebSocket, HTTP, and SSE connections
- 🔐 **Flexible Authentication**: Multiple auth methods including basic, bearer, and OAuth
- ⚡ **Real-time Updates**: Live connection monitoring and health checks

## Architecture

```
┌─────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Grafana   │◄──►│  MCP DataSource │◄──►│   MCP Server    │
│  Dashboard  │    │     Plugin      │    │ (External Tool) │
└─────────────┘    └─────────────────┘    └─────────────────┘
```

- **Grafana Dashboard**: Visualizes data and provides the user interface
- **MCP DataSource Plugin**: Translates between Grafana and MCP protocols
- **MCP Server**: External service exposing tools, data, or capabilities

## Getting Started

### Prerequisites

- Grafana 8.0+ 
- Go 1.24+
- Node.js 18+
- Access to one or more MCP servers

### Development Setup

1. **Clone and setup the project:**
   ```bash
   git clone <repository-url>
   cd grafana-mcpclient-datasource
   npm install
   go mod tidy
   ```

2. **Build the plugin:**
   ```bash
   # Build frontend
   npm run build
   
   # Build backend
   go build -o mcp-datasource ./pkg
   ```

3. **Run development server:**
   ```bash
   npm run server
   ```

### Configuration

1. **Add the datasource** in Grafana's Data Sources settings
2. **Configure MCP server connection:**
   - **Server URL**: Your MCP server endpoint (http, https, ws, wss)
   - **Transport**: Auto-detected based on URL scheme
   - **Authentication**: Configure credentials if required
   - **Timeouts**: Set connection and query timeouts

3. **Test the connection** using the built-in health check

### Query Types

The plugin supports multiple query types:

#### Natural Language Queries
```json
{
  "queryType": "natural_language",
  "query": "Show me the latest sales data from the database"
}
```

#### Direct Tool Calls
```json
{
  "queryType": "tool_call", 
  "toolName": "database_query",
  "toolArguments": "{\"table\": \"sales\", \"limit\": 100}"
}
```

#### Tool Discovery
```json
{
  "queryType": "list_tools"
}
```

## MCP Server Compatibility

This plugin works with any MCP-compliant server. Popular examples include:

- **Database Servers**: PostgreSQL, MySQL, SQLite MCP servers
- **File System Servers**: Local and remote file access
- **API Servers**: REST/GraphQL API wrappers
- **Cloud Services**: AWS, Azure, GCP integrations
- **Custom Tools**: Domain-specific business logic

## Development

### Backend Development

The backend is written in Go and uses the Grafana Plugin SDK:

```bash
# Install dependencies
go mod tidy

# Run tests
go test ./...

# Build
go build ./pkg
```

### Frontend Development

The frontend is built with React and TypeScript:

```bash
# Install dependencies
npm install

# Development mode
npm run dev

# Production build
npm run build

# Run tests
npm run test
```

### Project Structure

```
├── pkg/                    # Go backend code
│   ├── main.go            # Plugin entry point
│   ├── models/            # Data models and types
│   └── plugin/            # Plugin implementation
├── src/                   # TypeScript frontend code
│   ├── components/        # React components
│   ├── datasource.ts      # DataSource implementation
│   └── types.ts           # Type definitions
├── project/               # Project documentation
│   ├── plan.md           # Technical architecture
│   └── tasks.md          # Development tasks
└── provisioning/         # Grafana provisioning configs
```

## Contributing

Contributions are welcome! Please check the [development tasks](project/tasks.md) for current priorities.

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Submit a pull request

## Learn More

- [Model Context Protocol](https://modelcontextprotocol.io/) - Official MCP documentation
- [mcp-go Library](https://github.com/mark3labs/mcp-go) - The Go MCP library we use
- [Grafana Plugin Development](https://grafana.com/developers/plugin-tools/) - Official Grafana plugin docs
- [Project Plan](project/plan.md) - Detailed technical architecture
- [Development Tasks](project/tasks.md) - Current development status

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Acknowledgments

- Thanks to [mark3labs](https://github.com/mark3labs) for the excellent mcp-go library
- Thanks to the Grafana team for the plugin development framework
- Thanks to the MCP community for the protocol specification
