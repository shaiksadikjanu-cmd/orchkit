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

// WhatsApp sends messages via WhatsApp Business Cloud API.
// Get access at developers.facebook.com → WhatsApp → Getting Started.
//
// Example:
//
//	nodes.NewWhatsApp("access_token", "phone_number_id")
type WhatsApp struct {
	Token         string
	PhoneNumberID string
	client        *http.Client
}

func NewWhatsApp(token, phoneNumberID string) *WhatsApp {
	return &WhatsApp{Token: token, PhoneNumberID: phoneNumberID, client: &http.Client{Timeout: 15 * time.Second}}
}

func (w *WhatsApp) Name() string { return "whatsapp" }

func (w *WhatsApp) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends messages via WhatsApp Business Cloud API.",
		Params: map[string]any{
			"to":   map[string]any{"type": "string", "desc": "Recipient phone number with country code e.g. 15551234567"},
			"text": map[string]any{"type": "string", "desc": "Message text to send."},
		},
	}
}

func (w *WhatsApp) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	to, ok := in["to"].(string)
	if !ok || to == "" {
		return nil, fmt.Errorf("whatsapp: 'to' is required")
	}
	text, ok := in["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("whatsapp: 'text' is required")
	}

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]any{"body": text},
	}

	raw, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", w.PhoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("whatsapp: %w", err)
	}
	req.Header.Set("authorization", "Bearer "+w.Token)
	req.Header.Set("content-type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("whatsapp: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("whatsapp: api error %d: %s", resp.StatusCode, body)
	}

	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"sent": true, "result": result}, nil
}
