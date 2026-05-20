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

// geminiBackend drives the agent loop via Google Gemini API.
// Free tier: aistudio.google.com
// Note: Gemini tool-use works but has a different conversation format.
type geminiBackend struct {
	apiKey string
	model  string
	client *http.Client
}

func (g *geminiBackend) chat(ctx context.Context, system string, maxTokens int, tools []Tool, history []message) (*chatResponse, error) {
	if g.client == nil {
		g.client = &http.Client{Timeout: 60 * time.Second}
	}

	// Build Gemini contents array.
	contents := []map[string]any{}
	for _, m := range history {
		switch m.Role {
		case "user":
			contents = append(contents, map[string]any{
				"role":  "user",
				"parts": []map[string]any{{"text": m.Text}},
			})
		case "assistant":
			if len(m.ToolCalls) == 0 {
				contents = append(contents, map[string]any{
					"role":  "model",
					"parts": []map[string]any{{"text": m.Text}},
				})
			} else {
				parts := []map[string]any{}
				for _, tc := range m.ToolCalls {
					parts = append(parts, map[string]any{
						"functionCall": map[string]any{
							"name": tc.Name,
							"args": tc.Input,
						},
					})
				}
				contents = append(contents, map[string]any{"role": "model", "parts": parts})
			}
		case "tool":
			parts := []map[string]any{}
			for _, tr := range m.ToolResults {
				var result map[string]any
				_ = json.Unmarshal([]byte(tr.Content), &result)
				if result == nil {
					result = map[string]any{"output": tr.Content}
				}
				parts = append(parts, map[string]any{
					"functionResponse": map[string]any{
						"name":     tr.ID, // Gemini uses name not id here
						"response": result,
					},
				})
			}
			contents = append(contents, map[string]any{"role": "user", "parts": parts})
		}
	}

	// Build Gemini function declarations.
	funcDecls := []map[string]any{}
	for _, t := range tools {
		funcDecls = append(funcDecls, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters":  t.InputSchema,
		})
	}

	body := map[string]any{
		"contents": contents,
		"generationConfig": map[string]any{
			"maxOutputTokens": maxTokens,
		},
	}
	if system != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{{"text": system}},
		}
	}
	if len(funcDecls) > 0 {
		body["tools"] = []map[string]any{
			{"functionDeclarations": funcDecls},
		}
	}

	raw, _ := json.Marshal(body)
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		g.model, g.apiKey,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: api error %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text         *string        `json:"text"`
					FunctionCall *struct {
						Name string         `json:"name"`
						Args map[string]any `json:"args"`
					} `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("gemini: parsing response: %w", err)
	}
	if len(parsed.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: empty candidates")
	}

	cr := &chatResponse{}
	for _, part := range parsed.Candidates[0].Content.Parts {
		if part.Text != nil {
			cr.text += *part.Text
		}
		if part.FunctionCall != nil {
			cr.toolCalls = append(cr.toolCalls, toolCallRequest{
				ID:    part.FunctionCall.Name, // Gemini has no call ID, use name
				Name:  part.FunctionCall.Name,
				Input: part.FunctionCall.Args,
			})
		}
	}
	return cr, nil
}
