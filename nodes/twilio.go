package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Twilio sends SMS messages via the Twilio API.
//
// Example:
//
//	nodes.NewTwilio("AC_account_sid", "auth_token", "+1234567890")
type Twilio struct {
	AccountSID string
	AuthToken  string
	From       string
	client     *http.Client
}

func NewTwilio(accountSID, authToken, from string) *Twilio {
	return &Twilio{
		AccountSID: accountSID,
		AuthToken:  authToken,
		From:       from,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *Twilio) Name() string { return "twilio" }

func (t *Twilio) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends SMS messages via Twilio. Also supports WhatsApp with 'whatsapp:+1...' prefix.",
		Params: map[string]any{
			"to":   map[string]any{"type": "string", "desc": "Recipient phone number e.g. +1234567890."},
			"text": map[string]any{"type": "string", "desc": "Message body."},
			"from": map[string]any{"type": "string", "desc": "Sender number. Falls back to constructor."},
		},
	}
}

func (t *Twilio) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	to, ok := in["to"].(string)
	if !ok || to == "" {
		return nil, fmt.Errorf("twilio: 'to' is required")
	}
	text, ok := in["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("twilio: 'text' is required")
	}
	from := t.From
	if v, ok := in["from"].(string); ok && v != "" {
		from = v
	}

	params := url.Values{}
	params.Set("To", to)
	params.Set("From", from)
	params.Set("Body", text)

	apiURL := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Messages.json", t.AccountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("twilio: %w", err)
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(t.AccountSID, t.AuthToken)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twilio: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("twilio: api error %d: %s", resp.StatusCode, body)
	}

	var result struct {
		SID    string `json:"sid"`
		Status string `json:"status"`
	}
	json.Unmarshal(body, &result)

	return orchkit.Output{
		"sid":    result.SID,
		"status": result.Status,
		"sent":   true,
	}, nil
}
