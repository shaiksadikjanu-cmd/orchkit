package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// LLM calls the Anthropic /v1/messages endpoint.
// No SDK — just net/http + encoding/json. Stays dependency-free.
//
// Example:
//
//	nodes.NewLLM(apiKey, "claude-sonnet-4-20250514")
type LLM struct {
	APIKey string
	Model  string
	Client *http.Client
}

func NewLLM(apiKey, model string) *LLM {
	return &LLM{
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (l *LLM) Name() string { return "llm" }

func (l *LLM) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends a prompt to an Anthropic LLM and returns the text response.",
		Params: map[string]any{
			"prompt": map[string]any{
				"type": "string",
				"desc": "The user message to send to the model.",
			},
			"system": map[string]any{
				"type": "string",
				"desc": "Optional system prompt.",
			},
			"max_tokens": map[string]any{
				"type": "number",
				"desc": "Max tokens to generate. Defaults to 1024.",
			},
		},
	}
}

func (l *LLM) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	prompt, ok := in["prompt"].(string)
	if !ok || prompt == "" {
		return nil, fmt.Errorf("llm: input \"prompt\" is required")
	}

	model := l.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	maxTokens := 1024
	if v, ok := in["max_tokens"].(int); ok && v > 0 {
		maxTokens = v
	}

	body := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}

	if system, ok := in["system"].(string); ok && system != "" {
		body["system"] = system
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm: marshalling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm: building request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", l.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := l.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm: api error %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("llm: parsing response: %w", err)
	}

	text := ""
	for _, block := range parsed.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	return orchkit.Output{
		"text":          text,
		"input_tokens":  parsed.Usage.InputTokens,
		"output_tokens": parsed.Usage.OutputTokens,
	}, nil
}
