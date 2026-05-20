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

// GeminiLLM calls the Google Gemini API.
// Free tier available at aistudio.google.com
//
// Example:
//
//	nodes.NewGeminiLLM(apiKey, "gemini-2.0-flash")
type GeminiLLM struct {
	APIKey string
	Model  string
	Client *http.Client
}

func NewGeminiLLM(apiKey, model string) *GeminiLLM {
	return &GeminiLLM{
		APIKey: apiKey,
		Model:  model,
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (g *GeminiLLM) Name() string { return "llm_gemini" }

func (g *GeminiLLM) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends a prompt to Google Gemini and returns the text response. Free tier available.",
		Params: map[string]any{
			"prompt": map[string]any{
				"type": "string",
				"desc": "The user message to send.",
			},
			"system": map[string]any{
				"type": "string",
				"desc": "Optional system instruction.",
			},
			"max_tokens": map[string]any{
				"type": "integer",
				"desc": "Max tokens to generate. Defaults to 1024.",
			},
		},
	}
}

func (g *GeminiLLM) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	prompt, ok := in["prompt"].(string)
	if !ok || prompt == "" {
		return nil, fmt.Errorf("llm_gemini: input \"prompt\" is required")
	}

	model := g.Model
	if model == "" {
		model = "gemini-2.0-flash"
	}

	maxTokens := 1024
	if v, ok := in["max_tokens"].(int); ok && v > 0 {
		maxTokens = v
	}

	// Gemini request shape.
	body := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"maxOutputTokens": maxTokens,
		},
	}

	// System instruction is a top-level field in Gemini.
	if system, ok := in["system"].(string); ok && system != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]any{
				{"text": system},
			},
		}
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("llm_gemini: marshalling request: %w", err)
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, g.APIKey,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("llm_gemini: building request: %w", err)
	}
	req.Header.Set("content-type", "application/json")

	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm_gemini: http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("llm_gemini: reading response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm_gemini: api error %d: %s", resp.StatusCode, respBody)
	}

	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("llm_gemini: parsing response: %w", err)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("llm_gemini: empty response from api")
	}

	text := ""
	for _, part := range parsed.Candidates[0].Content.Parts {
		text += part.Text
	}

	return orchkit.Output{
		"text":          text,
		"input_tokens":  parsed.UsageMetadata.PromptTokenCount,
		"output_tokens": parsed.UsageMetadata.CandidatesTokenCount,
	}, nil
}
