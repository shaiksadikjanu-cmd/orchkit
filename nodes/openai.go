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

// OpenAI calls the OpenAI API.
// Actions: chat, embed, image.
// Works with GPT-4o, GPT-4, GPT-3.5, text-embedding-3, dall-e-3.
//
// Example:
//
//	nodes.NewOpenAI("sk-your-key", "gpt-4o")
type OpenAI struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewOpenAI(apiKey, model string) *OpenAI {
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAI{APIKey: apiKey, Model: model, client: &http.Client{Timeout: 120 * time.Second}}
}

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Calls OpenAI API. Actions: chat (default), embed, image.",
		Params: map[string]any{
			"prompt":     map[string]any{"type": "string", "desc": "User message or text to embed/generate image from."},
			"system":     map[string]any{"type": "string", "desc": "System prompt (chat only)."},
			"action":     map[string]any{"type": "string", "desc": "chat | embed | image. Defaults to chat."},
			"model":      map[string]any{"type": "string", "desc": "Model override e.g. gpt-4o, gpt-3.5-turbo."},
			"max_tokens": map[string]any{"type": "number", "desc": "Max tokens (chat). Default 1024."},
			"size":       map[string]any{"type": "string", "desc": "Image size (image): 1024x1024, 512x512. Default 1024x1024."},
		},
	}
}

func (o *OpenAI) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	if action == "" {
		action = "chat"
	}

	model := o.Model
	if v, ok := in["model"].(string); ok && v != "" {
		model = v
	}

	prompt, _ := in["prompt"].(string)
	if prompt == "" {
		return nil, fmt.Errorf("openai: prompt is required")
	}

	switch action {
	case "chat":
		maxTokens := 1024
		if v, ok := in["max_tokens"].(float64); ok && v > 0 {
			maxTokens = int(v)
		}
		messages := []map[string]any{{"role": "user", "content": prompt}}
		if sys, ok := in["system"].(string); ok && sys != "" {
			messages = append([]map[string]any{{"role": "system", "content": sys}}, messages...)
		}
		body := map[string]any{
			"model":      model,
			"messages":   messages,
			"max_tokens": maxTokens,
		}
		return o.call(ctx, "/v1/chat/completions", body, func(raw []byte) (orchkit.Output, error) {
			var resp struct {
				Choices []struct {
					Message struct{ Content string `json:"content"` } `json:"message"`
				} `json:"choices"`
				Usage struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return nil, err
			}
			if len(resp.Choices) == 0 {
				return nil, fmt.Errorf("no choices in response")
			}
			return orchkit.Output{
				"text":          resp.Choices[0].Message.Content,
				"input_tokens":  resp.Usage.PromptTokens,
				"output_tokens": resp.Usage.CompletionTokens,
			}, nil
		})

	case "embed":
		embedModel := model
		if embedModel == "gpt-4o" {
			embedModel = "text-embedding-3-small"
		}
		body := map[string]any{"model": embedModel, "input": prompt}
		return o.call(ctx, "/v1/embeddings", body, func(raw []byte) (orchkit.Output, error) {
			var resp struct {
				Data []struct {
					Embedding []float64 `json:"embedding"`
				} `json:"data"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return nil, err
			}
			if len(resp.Data) == 0 {
				return nil, fmt.Errorf("no embedding in response")
			}
			return orchkit.Output{
				"embedding": resp.Data[0].Embedding,
				"dims":      len(resp.Data[0].Embedding),
			}, nil
		})

	case "image":
		size, _ := in["size"].(string)
		if size == "" {
			size = "1024x1024"
		}
		body := map[string]any{
			"model":  "dall-e-3",
			"prompt": prompt,
			"n":      1,
			"size":   size,
		}
		return o.call(ctx, "/v1/images/generations", body, func(raw []byte) (orchkit.Output, error) {
			var resp struct {
				Data []struct{ URL string `json:"url"` } `json:"data"`
			}
			if err := json.Unmarshal(raw, &resp); err != nil {
				return nil, err
			}
			if len(resp.Data) == 0 {
				return nil, fmt.Errorf("no image in response")
			}
			return orchkit.Output{"url": resp.Data[0].URL}, nil
		})

	default:
		return nil, fmt.Errorf("openai: unknown action %q", action)
	}
}

func (o *OpenAI) call(ctx context.Context, path string, body map[string]any, parse func([]byte) (orchkit.Output, error)) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com"+path, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	req.Header.Set("authorization", "Bearer "+o.APIKey)
	req.Header.Set("content-type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("openai: api error %d: %s", resp.StatusCode, respBody)
	}
	return parse(respBody)
}
