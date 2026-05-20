package ai_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"fmt"
	"testing"

	"orchkit"
	"orchkit/ai"
)

// fakeNode for MCP tests — no real I/O
type fakeNode struct {
	name   string
	output orchkit.Output
	err    error
}

func (f *fakeNode) Name() string           { return f.name }
func (f *fakeNode) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "A fake node for testing.",
		Params: map[string]any{
			"input": map[string]any{"type": "string", "desc": "test input"},
		},
	}
}
func (f *fakeNode) Execute(_ context.Context, _ orchkit.Input) (orchkit.Output, error) {
	return f.output, f.err
}

func mcpRPC(t *testing.T, server *ai.MCPServer, method string, params any) map[string]any {
	t.Helper()
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	raw, _ := json.Marshal(req)

	var out bytes.Buffer
	in := bytes.NewReader(append(raw, '\n'))
	_ = server.ServeIO(context.Background(), in, &out)

	var resp map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal response: %v — raw: %s", err, out.String())
	}
	return resp
}

func TestMCP_Initialize(t *testing.T) {
	server := ai.NewMCPServer()
	resp := mcpRPC(t, server, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
	})

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got: %v", resp)
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Fatalf("wrong protocol version: %v", result["protocolVersion"])
	}
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "orchkit" {
		t.Fatalf("wrong server name: %v", info["name"])
	}
}

func TestMCP_ToolsList(t *testing.T) {
	node := &fakeNode{name: "test_node", output: orchkit.Output{"ok": true}}
	server := ai.NewMCPServer(node)
	resp := mcpRPC(t, server, "tools/list", map[string]any{})

	result, _ := resp["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool, _ := tools[0].(map[string]any)
	if tool["name"] != "test_node" {
		t.Fatalf("wrong tool name: %v", tool["name"])
	}
	if tool["description"] != "A fake node for testing." {
		t.Fatalf("wrong description: %v", tool["description"])
	}
}

func TestMCP_ToolsCall_Success(t *testing.T) {
	node := &fakeNode{
		name:   "echo",
		output: orchkit.Output{"result": "it works"},
	}
	server := ai.NewMCPServer(node)
	resp := mcpRPC(t, server, "tools/call", map[string]any{
		"name":      "echo",
		"arguments": map[string]any{"input": "test"},
	})

	result, _ := resp["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("expected content in response")
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	if !strings.Contains(text, "it works") {
		t.Fatalf("expected 'it works' in response, got: %s", text)
	}
}

func TestMCP_ToolsCall_NodeError(t *testing.T) {
	node := &fakeNode{
		name: "broken",
		err:  fmt.Errorf("something went wrong"),
	}
	server := ai.NewMCPServer(node)
	resp := mcpRPC(t, server, "tools/call", map[string]any{
		"name":      "broken",
		"arguments": map[string]any{},
	})

	result, _ := resp["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatal("expected isError=true for node error")
	}
	content, _ := result["content"].([]any)
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	if !strings.Contains(text, "something went wrong") {
		t.Fatalf("expected error message in content, got: %s", text)
	}
}

func TestMCP_ToolsCall_UnknownTool(t *testing.T) {
	server := ai.NewMCPServer()
	resp := mcpRPC(t, server, "tools/call", map[string]any{
		"name":      "nonexistent",
		"arguments": map[string]any{},
	})

	// Unknown tool returns isError=true in result (not a JSON-RPC error)
	result, _ := resp["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("expected isError=true for unknown tool, got: %v", resp)
	}
}

func TestMCP_UnknownMethod(t *testing.T) {
	server := ai.NewMCPServer()
	resp := mcpRPC(t, server, "does/not/exist", map[string]any{})

	errBlock, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error block for unknown method, got: %v", resp)
	}
	if errBlock["code"] != float64(-32601) {
		t.Fatalf("expected code -32601, got: %v", errBlock["code"])
	}
}

func TestMCP_MultipleNodes(t *testing.T) {
	nodes := []orchkit.Node{
		&fakeNode{name: "node_a", output: orchkit.Output{"a": 1}},
		&fakeNode{name: "node_b", output: orchkit.Output{"b": 2}},
		&fakeNode{name: "node_c", output: orchkit.Output{"c": 3}},
	}
	server := ai.NewMCPServer(nodes...)
	resp := mcpRPC(t, server, "tools/list", map[string]any{})

	result, _ := resp["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
}
