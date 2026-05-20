package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"orchkit"
)

// ----------------------------------------------------------------------------
// Agent — a tool-use loop that lets Claude drive your nodes.
//
// How it works:
//  1. You give Agent a task (plain English) and a list of nodes.
//  2. Agent converts the nodes to tools and sends them to Claude.
//  3. Claude either responds with text (done) or calls a tool (node).
//  4. Agent executes the node, feeds the result back to Claude.
//  5. Repeat until Claude stops calling tools.
//
// The loop is deliberately simple. No streaming, no parallel tool calls,
// no memory between runs. Add those only when you actually need them.
// ----------------------------------------------------------------------------

// AgentConfig holds everything the agent needs. No global state.
type AgentConfig struct {
	APIKey    string
	Model     string
	MaxTokens int
	MaxTurns  int       // safety cap — stops infinite loops
	Nodes     []orchkit.Node
	System    string    // optional system prompt
}

// Result is what the agent returns when it's done.
type Result struct {
	Text  string            // Claude's final text response
	Turns int               // how many tool-call rounds happened
	Calls []ToolCallRecord  // every node call that was made
}

// ToolCallRecord logs one node execution for inspection.
type ToolCallRecord struct {
	Node   string
	Input  orchkit.Input
	Output orchkit.Output
	Err    error
}

// Run executes the agent loop.
func Run(ctx context.Context, cfg AgentConfig, task string) (*Result, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("agent: APIKey is required")
	}
	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-20250514"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 10
	}

	tools := Tools(cfg.Nodes...)
	client := &http.Client{Timeout: 60 * time.Second}

	// messages is the full conversation history sent on every turn.
	messages := []map[string]any{
		{"role": "user", "content": task},
	}

	result := &Result{}

	for turn := 0; turn < cfg.MaxTurns; turn++ {
		resp, err := callClaude(ctx, client, cfg, tools, messages)
		if err != nil {
			return nil, fmt.Errorf("agent turn %d: %w", turn+1, err)
		}

		// Append Claude's response to history.
		messages = append(messages, map[string]any{
			"role":    "assistant",
			"content": resp.Content,
		})

		// Check what Claude wants to do.
		toolCalls := extractToolCalls(resp.Content)

		if len(toolCalls) == 0 {
			// No tool calls — Claude is done. Extract the final text.
			result.Turns = turn + 1
			for _, block := range resp.Content {
				if block["type"] == "text" {
					result.Text += fmt.Sprint(block["text"])
				}
			}
			return result, nil
		}

		// Execute every tool call Claude requested and collect results.
		toolResults := []map[string]any{}
		for _, call := range toolCalls {
			out, err := Dispatch(ctx, cfg.Nodes, call.name, call.input)

			record := ToolCallRecord{
				Node:   call.name,
				Input:  call.input,
				Output: out,
				Err:    err,
			}
			result.Calls = append(result.Calls, record)

			content := ""
			if err != nil {
				content = fmt.Sprintf("error: %v", err)
			} else {
				b, _ := json.Marshal(out)
				content = string(b)
			}

			toolResults = append(toolResults, map[string]any{
				"type":        "tool_result",
				"tool_use_id": call.id,
				"content":     content,
			})
		}

		// Feed tool results back as a user turn.
		messages = append(messages, map[string]any{
			"role":    "user",
			"content": toolResults,
		})
	}

	return nil, fmt.Errorf("agent: exceeded max turns (%d)", cfg.MaxTurns)
}

// ----------------------------------------------------------------------------
// Internal helpers
// ----------------------------------------------------------------------------

type claudeResponse struct {
	Content    []map[string]any `json:"content"`
	StopReason string           `json:"stop_reason"`
}

type toolCall struct {
	id    string
	name  string
	input orchkit.Input
}

func callClaude(ctx context.Context, client *http.Client, cfg AgentConfig, tools []Tool, messages []map[string]any) (*claudeResponse, error) {
	// Convert tools to the shape Anthropic expects.
	apiTools := make([]map[string]any, len(tools))
	for i, t := range tools {
		apiTools[i] = map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.InputSchema,
		}
	}

	body := map[string]any{
		"model":      cfg.Model,
		"max_tokens": cfg.MaxTokens,
		"tools":      apiTools,
		"messages":   messages,
	}
	if cfg.System != "" {
		body["system"] = cfg.System
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error %d: %s", resp.StatusCode, respBody)
	}

	var parsed claudeResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func extractToolCalls(content []map[string]any) []toolCall {
	var calls []toolCall
	for _, block := range content {
		if block["type"] != "tool_use" {
			continue
		}
		id, _ := block["id"].(string)
		name, _ := block["name"].(string)
		input, _ := block["input"].(map[string]any)
		calls = append(calls, toolCall{id: id, name: name, input: input})
	}
	return calls
}
