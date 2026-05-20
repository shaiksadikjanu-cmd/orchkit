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

// anthropicBackend drives the agent loop via Anthropic Claude API.
type anthropicBackend struct {
	apiKey string
	model  string
	client *http.Client
}

func (a *anthropicBackend) chat(ctx context.Context, system string, maxTokens int, tools []Tool, history []message) (*chatResponse, error) {
	if a.client == nil {
		a.client = &http.Client{Timeout: 60 * time.Second}
	}

	msgs := []map[string]any{}
	for _, m := range history {
		switch m.Role {
		case "user":
			msgs = append(msgs, map[string]any{"role": "user", "content": m.Text})
		case "assistant":
			if len(m.ToolCalls) == 0 {
				msgs = append(msgs, map[string]any{"role": "assistant", "content": m.Text})
			} else {
				content := []map[string]any{}
				for _, tc := range m.ToolCalls {
					content = append(content, map[string]any{
						"type":  "tool_use",
						"id":    tc.ID,
						"name":  tc.Name,
						"input": tc.Input,
					})
				}
				msgs = append(msgs, map[string]any{"role": "assistant", "content": content})
			}
		case "tool":
			content := []map[string]any{}
			for _, tr := range m.ToolResults {
				content = append(content, map[string]any{
					"type":        "tool_result",
					"tool_use_id": tr.ID,
					"content":     tr.Content,
				})
			}
			msgs = append(msgs, map[string]any{"role": "user", "content": content})
		}
	}

	apiTools := []map[string]any{}
	for _, t := range tools {
		apiTools = append(apiTools, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": t.InputSchema,
		})
	}

	body := map[string]any{
		"model":      a.model,
		"max_tokens": maxTokens,
		"messages":   msgs,
		"tools":      apiTools,
	}
	if system != "" {
		body["system"] = system
	}

	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: api error %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		Content []struct {
			Type  string         `json:"type"`
			Text  string         `json:"text"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("anthropic: parsing response: %w", err)
	}

	cr := &chatResponse{}
	for _, block := range parsed.Content {
		switch block.Type {
		case "text":
			cr.text += block.Text
		case "tool_use":
			cr.toolCalls = append(cr.toolCalls, toolCallRequest{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}
	return cr, nil
}
