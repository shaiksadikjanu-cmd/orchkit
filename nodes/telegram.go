package nodes

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

// Telegram sends a message via the Telegram Bot API.
// Get a bot token from @BotFather. Get your chat ID from @userinfobot.
//
// Example:
//
//	nodes.NewTelegram("bot-token", "chat-id")
type Telegram struct {
	Token  string
	ChatID string
	client *http.Client
}

func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{Token: token, ChatID: chatID, client: &http.Client{Timeout: 15 * time.Second}}
}

func (t *Telegram) Name() string { return "telegram" }

func (t *Telegram) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends a message via Telegram Bot API. Supports plain text and HTML/Markdown formatting.",
		Params: map[string]any{
			"text":       map[string]any{"type": "string", "desc": "Message text."},
			"chat_id":    map[string]any{"type": "string", "desc": "Telegram chat ID. Falls back to constructor value."},
			"parse_mode": map[string]any{"type": "string", "desc": "Formatting: HTML, Markdown, or empty for plain text."},
		},
	}
}

func (t *Telegram) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	text, ok := in["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("telegram: 'text' is required")
	}

	chatID := t.ChatID
	if v, ok := in["chat_id"].(string); ok && v != "" {
		chatID = v
	}
	if chatID == "" {
		return nil, fmt.Errorf("telegram: chat_id is required")
	}

	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if mode, ok := in["parse_mode"].(string); ok && mode != "" {
		payload["parse_mode"] = mode
	}

	raw, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	req.Header.Set("content-type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			MessageID int `json:"message_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("telegram: parse response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram: api error: %s", result.Description)
	}

	return orchkit.Output{
		"sent":       true,
		"message_id": result.Result.MessageID,
		"chat_id":    chatID,
	}, nil
}
