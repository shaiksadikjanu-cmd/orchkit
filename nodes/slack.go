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

// Slack sends a message to a Slack channel via webhook or Bot token.
//
// Via webhook (simplest):
//
//	nodes.NewSlack("").WithWebhook("https://hooks.slack.com/services/...")
//
// Via Bot token:
//
//	nodes.NewSlack("xoxb-your-token")
type Slack struct {
	Token      string
	WebhookURL string
	Channel    string
	client     *http.Client
}

func NewSlack(token string) *Slack {
	return &Slack{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *Slack) WithWebhook(url string) *Slack {
	s.WebhookURL = url
	return s
}

func (s *Slack) WithChannel(ch string) *Slack {
	s.Channel = ch
	return s
}

func (s *Slack) Name() string { return "slack" }

func (s *Slack) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends a message to a Slack channel via webhook URL or Bot token.",
		Params: map[string]any{
			"text":    map[string]any{"type": "string", "desc": "Message text to send."},
			"channel": map[string]any{"type": "string", "desc": "Channel name or ID (Bot token mode only)."},
			"webhook": map[string]any{"type": "string", "desc": "Slack webhook URL (overrides constructor)."},
		},
	}
}

func (s *Slack) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	text, ok := in["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("slack: 'text' is required")
	}

	webhook := s.WebhookURL
	if v, ok := in["webhook"].(string); ok && v != "" {
		webhook = v
	}

	channel := s.Channel
	if v, ok := in["channel"].(string); ok && v != "" {
		channel = v
	}

	var (
		url     string
		payload map[string]any
		headers map[string]string
	)

	if webhook != "" {
		url = webhook
		payload = map[string]any{"text": text}
		headers = map[string]string{"content-type": "application/json"}
	} else if s.Token != "" {
		url = "https://slack.com/api/chat.postMessage"
		payload = map[string]any{"channel": channel, "text": text}
		headers = map[string]string{
			"content-type":  "application/json",
			"authorization": "Bearer " + s.Token,
		}
	} else {
		return nil, fmt.Errorf("slack: provide webhook URL or bot token")
	}

	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("slack: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("slack: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("slack: api error %d: %s", resp.StatusCode, body)
	}

	return orchkit.Output{"sent": true, "channel": channel}, nil
}
