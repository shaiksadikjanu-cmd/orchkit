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

// GroqLLM calls the Groq API (OpenAI-compatible endpoint).
// Free tier available at console.groq.com
// Fast inference on Llama, Mixtral, Gemma models.
//
// Example:
//
//	nodes.NewGroqLLM(apiKey, "llama-3.3-70b-versatile")
type GroqLLM struct {
	APIKey string
	Model  string
	Client *http.Client
}

func NewGroqLLM(apiKey, model string) *GroqLLM {
	return &GroqLLM{
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *GroqLLM) Name() string { return "llm_groq" }

func (g *GroqLLM) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends a prompt to a Groq-hosted LLM and returns the text response. Free tier available.",
		Params: map[string]any{
			"prompt": map[string]any{
				"type": "string",
				"desc": "The user message to send.",
			},
			"system": map[string]any{
				"type": "string",
				"desc": "Optional system prompt.",
			},
			"max_tokens": map[string]any{
				"type": "integer",
				"desc": "Max tokens to generate. Defaults to 1024.",
			},
		},
	}
}

func (g *GroqLLM) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	prompt, ok := in["prompt"].(string)
	if !ok || prompt == "" {
		return nil, fmt.Errorf("llm_groq: input \"prompt\" is required")
	}

	model := g.Model
	if model == "" {
		model = "llama-3.3-70b-versatile"
	}

	maxTokens := 1024
	if v, ok := in["max_tokens"].(int); ok && v > 0 {
		maxTokens = v
	}

	messages := []map[string]any{}
	if system, ok := in["system"].(string); ok && system != "" {
		messages = append(messages, map[string]any{
			"role":    "system",
			"content": system,
		})
	}
	messages = append(messages, map[string]any{
		"role":    "user",
		"content": prompt,
	})

	body := map[string]any{
		"model":      model,
		"max_tokens": maxTokens,
		"messages":   messages,
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm_groq: marshalling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm_groq: building request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+g.APIKey)

	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm_groq: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm_groq: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm_groq: api error %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("llm_groq: parsing response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("llm_groq: empty choices in response")
	}

	return orchkit.Output{
		"text":          parsed.Choices[0].Message.Content,
		"input_tokens":  parsed.Usage.PromptTokens,
		"output_tokens": parsed.Usage.CompletionTokens,
	}, nil
}
