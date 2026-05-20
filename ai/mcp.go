package ai

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"orchkit"
)

// ----------------------------------------------------------------------------
// MCP Server — exposes orchkit nodes as an MCP (Model Context Protocol) server.
//
// MCP uses JSON-RPC 2.0 over stdin/stdout. This server implements:
//   - initialize        — handshake with the MCP client
//   - tools/list        — returns all registered nodes as tools
//   - tools/call        — executes a node by name with given input
//
// Usage:
//   server := ai.NewMCPServer(nodes...)
//   server.Serve(ctx)   // blocks, reads from stdin, writes to stdout
//
// Wire it up in Claude Desktop's config:
//   {
//     "mcpServers": {
//       "orchkit": {
//         "command": "/path/to/orchkit-mcp"
//       }
//     }
//   }
// ----------------------------------------------------------------------------

// MCPServer serves orchkit nodes over the MCP protocol.
type MCPServer struct {
	nodes  []orchkit.Node
	tools  []Tool
	logger *log.Logger
}

// NewMCPServer creates a server exposing the given nodes as MCP tools.
func NewMCPServer(nodes ...orchkit.Node) *MCPServer {
	return &MCPServer{
		nodes:  nodes,
		tools:  Tools(nodes...),
		logger: log.New(os.Stderr, "[orchkit-mcp] ", log.LstdFlags),
	}
}

// Serve starts the MCP server. Blocks until ctx is cancelled or stdin closes.
// All MCP communication is over stdin/stdout. Logs go to stderr.
func (s *MCPServer) Serve(ctx context.Context) error {
	return s.ServeIO(ctx, os.Stdin, os.Stdout)
}

// serveIO is the testable core — accepts any reader/writer.
func (s *MCPServer) ServeIO(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB max message

	s.logger.Printf("orchkit MCP server started with %d node(s)", len(s.nodes))
	for _, n := range s.nodes {
		s.logger.Printf("  tool: %s — %s", n.Name(), n.Schema().Description)
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Printf("bad request: %v", err)
			s.writeError(out, nil, -32700, "parse error", err.Error())
			continue
		}

		s.logger.Printf("← %s (id=%v)", req.Method, req.ID)
		resp := s.handle(ctx, &req)
		s.logger.Printf("→ %s (id=%v)", req.Method, req.ID)

		raw, err := json.Marshal(resp)
		if err != nil {
			s.logger.Printf("marshal error: %v", err)
			continue
		}
		fmt.Fprintf(out, "%s\n", raw)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("mcp: scanner: %w", err)
	}
	return nil
}

func (s *MCPServer) handle(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		return nil // no response needed for notifications
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(ctx, req)
	default:
		return errorResponse(req.ID, -32601, "method not found", req.Method)
	}
}

func (s *MCPServer) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "orchkit",
				"version": "0.1.0",
			},
		},
	}
}

func (s *MCPServer) handleToolsList(req *jsonRPCRequest) *jsonRPCResponse {
	mcpTools := make([]map[string]any, len(s.tools))
	for i, t := range s.tools {
		mcpTools[i] = map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.InputSchema,
		}
	}
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  map[string]any{"tools": mcpTools},
	}
}

func (s *MCPServer) handleToolsCall(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	// Parse params: {"name": "node_name", "arguments": {...}}
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	raw, _ := json.Marshal(req.Params)
	if err := json.Unmarshal(raw, &params); err != nil {
		return errorResponse(req.ID, -32602, "invalid params", err.Error())
	}
	if params.Name == "" {
		return errorResponse(req.ID, -32602, "invalid params", "name is required")
	}

	out, err := Dispatch(ctx, s.nodes, params.Name, params.Arguments)
	if err != nil {
		// MCP tool errors are returned as content with isError=true, not JSON-RPC errors.
		return &jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": fmt.Sprintf("error: %v", err)},
				},
				"isError": true,
			},
		}
	}

	resultJSON, _ := json.MarshalIndent(out, "", "  ")
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": string(resultJSON)},
			},
		},
	}
}

func (s *MCPServer) writeError(out io.Writer, id any, code int, message, data string) {
	resp := errorResponse(id, code, message, data)
	raw, _ := json.Marshal(resp)
	fmt.Fprintf(out, "%s\n", raw)
}

// ----------------------------------------------------------------------------
// JSON-RPC 2.0 types
// ----------------------------------------------------------------------------

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func errorResponse(id any, code int, message, data string) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: map[string]any{
			"code":    code,
			"message": message,
			"data":    data,
		},
	}
}
