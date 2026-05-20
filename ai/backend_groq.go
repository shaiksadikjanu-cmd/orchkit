package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// groqBackend drives the agent loop via Groq's OpenAI-compatible API.
// Free tier: console.groq.com — supports tool-use with Llama 3.3.
type groqBackend struct {
	apiKey string
	model  string
	client *http.Client
}

func (g *groqBackend) chat(ctx context.Context, system string, maxTokens int, tools []Tool, history []message) (*chatResponse, error) {
	if g.client == nil {
		g.client = &http.Client{Timeout: 60 * time.Second}
	}

	// Build messages array.
	msgs := []map[string]any{}
	if system != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": system})
	}

	for _, m := range history {
		switch m.Role {
		case "user":
			msgs = append(msgs, map[string]any{"role": "user", "content": m.Text})
		case "assistant":
			if len(m.ToolCalls) == 0 {
				msgs = append(msgs, map[string]any{"role": "assistant", "content": m.Text})
			} else {
				calls := []map[string]any{}
				for _, tc := range m.ToolCalls {
					arg, _ := json.Marshal(tc.Input)
					calls = append(calls, map[string]any{
						"id":   tc.ID,
						"type": "function",
						"function": map[string]any{
							"name":      tc.Name,
							"arguments": string(arg),
						},
					})
				}
				msgs = append(msgs, map[string]any{
					"role":       "assistant",
					"content":    nil,
					"tool_calls": calls,
				})
			}
		case "tool":
			for _, tr := range m.ToolResults {
				msgs = append(msgs, map[string]any{
					"role":         "tool",
					"tool_call_id": tr.ID,
					"content":      tr.Content,
				})
			}
		}
	}

	// Build tools array (OpenAI format).
	apiTools := []map[string]any{}
	for _, t := range tools {
		apiTools = append(apiTools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.InputSchema,
			},
		})
	}

	body := map[string]any{
		"model":      g.model,
		"max_tokens": maxTokens,
		"messages":   msgs,
	}
	if len(apiTools) > 0 {
		body["tools"] = apiTools
	}

	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+g.apiKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("groq: api error %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content   *string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("groq: parsing response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("groq: empty choices")
	}

	msg := parsed.Choices[0].Message
	cr := &chatResponse{}
	if msg.Content != nil {
		cr.text = *msg.Content
	}
	for _, tc := range msg.ToolCalls {
		var input map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		cr.toolCalls = append(cr.toolCalls, toolCallRequest{
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}
	return cr, nil
}
