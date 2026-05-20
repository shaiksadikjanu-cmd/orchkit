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

// Discord sends messages to a Discord channel via webhook or Bot token.
// Supports plain text, embeds, and username/avatar overrides.
//
// Example (webhook — simplest):
//
//	nodes.NewDiscord("").WithWebhook("https://discord.com/api/webhooks/...")
//
// Example (Bot token):
//
//	nodes.NewDiscord("your-bot-token")
type Discord struct {
	Token      string
	WebhookURL string
	client     *http.Client
}

func NewDiscord(token string) *Discord {
	return &Discord{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (d *Discord) WithWebhook(url string) *Discord {
	d.WebhookURL = url
	return d
}

func (d *Discord) Name() string { return "discord" }

func (d *Discord) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends a message to a Discord channel via webhook or Bot token.",
		Params: map[string]any{
			"text":       map[string]any{"type": "string", "desc": "Message text (up to 2000 chars)."},
			"channel_id": map[string]any{"type": "string", "desc": "Channel ID (bot token mode only)."},
			"webhook":    map[string]any{"type": "string", "desc": "Discord webhook URL."},
			"username":   map[string]any{"type": "string", "desc": "Override bot display name (webhook only)."},
			"embed":      map[string]any{"type": "object", "desc": "Optional embed object {title, description, color}."},
		},
	}
}

func (d *Discord) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	text, _ := in["text"].(string)
	webhook := d.WebhookURL
	if v, ok := in["webhook"].(string); ok && v != "" {
		webhook = v
	}

	var url string
	payload := map[string]any{}

	if webhook != "" {
		url = webhook
		if text != "" {
			payload["content"] = text
		}
		if u, ok := in["username"].(string); ok && u != "" {
			payload["username"] = u
		}
		if embed, ok := in["embed"].(map[string]any); ok {
			payload["embeds"] = []any{embed}
		}
	} else if d.Token != "" {
		channelID, _ := in["channel_id"].(string)
		if channelID == "" {
			return nil, fmt.Errorf("discord: channel_id required with bot token")
		}
		url = fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)
		payload["content"] = text
	} else {
		return nil, fmt.Errorf("discord: provide webhook URL or bot token")
	}

	if len(payload) == 0 {
		return nil, fmt.Errorf("discord: message content is required")
	}

	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("discord: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	if d.Token != "" {
		req.Header.Set("authorization", "Bot "+d.Token)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discord: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("discord: api error %d: %s", resp.StatusCode, body)
	}

	return orchkit.Output{"sent": true, "status": resp.StatusCode}, nil
}
