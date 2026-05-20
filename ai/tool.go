// Package ai exposes nodes as AI-callable tools.
// One adapter, ~50 lines. Same node, two lives.
package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"orchkit"
)

// Tool is a portable, JSON-serializable description of a Node.
// It maps cleanly to Anthropic's tool-use schema (and OpenAI's, and MCP's).
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// FromNode adapts a Node into a Tool description.
func FromNode(n orchkit.Node) Tool {
	s := n.Schema()
	return Tool{
		Name:        n.Name(),
		Description: s.Description,
		InputSchema: map[string]any{
			"type":       "object",
			"properties": s.Params,
		},
	}
}

// Tools adapts many nodes at once.
func Tools(ns ...orchkit.Node) []Tool {
	out := make([]Tool, len(ns))
	for i, n := range ns {
		out[i] = FromNode(n)
	}
	return out
}

// Dispatch invokes the node whose Name matches `name` with the given input.
// This is what an AI agent's tool-use loop calls.
func Dispatch(ctx context.Context, nodes []orchkit.Node, name string, in orchkit.Input) (orchkit.Output, error) {
	for _, n := range nodes {
		if n.Name() == name {
			return n.Execute(ctx, in)
		}
	}
	return nil, fmt.Errorf("ai: no node named %q", name)
}

// JSON is a convenience: marshal the tool list for sending to an API.
func JSON(tools []Tool) (string, error) {
	b, err := json.MarshalIndent(tools, "", "  ")
	return string(b), err
}
