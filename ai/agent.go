package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"orchkit"
)

// ----------------------------------------------------------------------------
// Agent — a tool-use loop driven by any supported backend.
//
// Backends: Groq (free, OpenAI-compatible), Gemini, Anthropic.
// Set whichever API key you have. Groq is recommended for free tier.
// ----------------------------------------------------------------------------

type AgentConfig struct {
	// Set one of these — whichever you have.
	GroqAPIKey     string
	GeminiAPIKey   string
	AnthropicAPIKey string

	Model     string         // optional: override default model per backend
	MaxTokens int            // defaults to 4096
	MaxTurns  int            // safety cap, defaults to 10
	Nodes     []orchkit.Node
	System    string
}

type Result struct {
	Text  string
	Turns int
	Calls []ToolCallRecord
}

type ToolCallRecord struct {
	Node   string
	Input  orchkit.Input
	Output orchkit.Output
	Err    error
}

// Run picks the available backend and executes the agent loop.
func Run(ctx context.Context, cfg AgentConfig, task string) (*Result, error) {
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	if cfg.MaxTurns == 0 {
		cfg.MaxTurns = 10
	}

	var backend backend
	switch {
	case cfg.GroqAPIKey != "":
		model := cfg.Model
		if model == "" {
			model = "llama-3.3-70b-versatile"
		}
		backend = &groqBackend{apiKey: cfg.GroqAPIKey, model: model}
	case cfg.GeminiAPIKey != "":
		model := cfg.Model
		if model == "" {
			model = "gemini-2.0-flash"
		}
		backend = &geminiBackend{apiKey: cfg.GeminiAPIKey, model: model}
	case cfg.AnthropicAPIKey != "":
		model := cfg.Model
		if model == "" {
			model = "claude-sonnet-4-20250514"
		}
		backend = &anthropicBackend{apiKey: cfg.AnthropicAPIKey, model: model}
	default:
		return nil, fmt.Errorf("agent: set at least one of GroqAPIKey, GeminiAPIKey, AnthropicAPIKey")
	}

	tools := Tools(cfg.Nodes...)
	result := &Result{}

	// Seed the conversation.
	history := []message{
		{Role: "user", Text: task},
	}

	for turn := 0; turn < cfg.MaxTurns; turn++ {
		resp, err := backend.chat(ctx, cfg.System, cfg.MaxTokens, tools, history)
		if err != nil {
			return nil, fmt.Errorf("agent turn %d: %w", turn+1, err)
		}

		history = append(history, message{Role: "assistant", Text: resp.text, ToolCalls: resp.toolCalls})

		if len(resp.toolCalls) == 0 {
			result.Text = resp.text
			result.Turns = turn + 1
			return result, nil
		}

		// Execute each tool call and collect results.
		var toolResults []toolResult
		for _, call := range resp.toolCalls {
			out, err := Dispatch(ctx, cfg.Nodes, call.Name, call.Input)
			result.Calls = append(result.Calls, ToolCallRecord{
				Node: call.Name, Input: call.Input, Output: out, Err: err,
			})

			content := ""
			if err != nil {
				content = fmt.Sprintf("error: %v", err)
			} else {
				b, _ := json.Marshal(out)
				content = string(b)
			}
			toolResults = append(toolResults, toolResult{ID: call.ID, Content: content})
		}

		history = append(history, message{Role: "tool", ToolResults: toolResults})
	}

	return nil, fmt.Errorf("agent: exceeded max turns (%d)", cfg.MaxTurns)
}

// ----------------------------------------------------------------------------
// Internal types shared across backends.
// ----------------------------------------------------------------------------

type toolCallRequest struct {
	ID    string
	Name  string
	Input orchkit.Input
}

type toolResult struct {
	ID      string
	Content string
}

type message struct {
	Role        string            // user | assistant | tool
	Text        string
	ToolCalls   []toolCallRequest
	ToolResults []toolResult
}

type chatResponse struct {
	text      string
	toolCalls []toolCallRequest
}

type backend interface {
	chat(ctx context.Context, system string, maxTokens int, tools []Tool, history []message) (*chatResponse, error)
}
