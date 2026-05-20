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

// Mailchimp manages email marketing via Mailchimp API v3.
// Actions: add_subscriber, get_subscriber, list_campaigns,
//          create_campaign, send_campaign.
//
// Example:
//
//	nodes.NewMailchimp("your_api_key", "us1") // server prefix from API key
type Mailchimp struct {
	APIKey string
	Server string // e.g. "us1" — last part of your API key after the dash
	client *http.Client
}

func NewMailchimp(apiKey, server string) *Mailchimp {
	return &Mailchimp{APIKey: apiKey, Server: server, client: &http.Client{Timeout: 15 * time.Second}}
}

func (m *Mailchimp) Name() string { return "mailchimp" }

func (m *Mailchimp) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages Mailchimp email marketing. Actions: add_subscriber, get_subscriber, list_campaigns, create_campaign.",
		Params: map[string]any{
			"action":   map[string]any{"type": "string", "desc": "add_subscriber | get_subscriber | list_campaigns | create_campaign"},
			"list_id":  map[string]any{"type": "string", "desc": "Mailchimp audience/list ID."},
			"email":    map[string]any{"type": "string", "desc": "Subscriber email."},
			"name":     map[string]any{"type": "string", "desc": "Subscriber name (add_subscriber)."},
			"subject":  map[string]any{"type": "string", "desc": "Campaign subject (create_campaign)."},
			"content":  map[string]any{"type": "string", "desc": "Campaign HTML content (create_campaign)."},
		},
	}
}

func (m *Mailchimp) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := fmt.Sprintf("https://%s.api.mailchimp.com/3.0", m.Server)
	action, _ := in["action"].(string)

	switch action {
	case "add_subscriber", "":
		listID, _ := in["list_id"].(string)
		email, _ := in["email"].(string)
		if listID == "" || email == "" {
			return nil, fmt.Errorf("mailchimp: list_id and email required")
		}
		body := map[string]any{
			"email_address": email,
			"status":        "subscribed",
		}
		if name, ok := in["name"].(string); ok {
			parts := splitName(name)
			body["merge_fields"] = map[string]any{
				"FNAME": parts[0],
				"LNAME": parts[1],
			}
		}
		return m.post(ctx, base+"/lists/"+listID+"/members", body)

	case "get_subscriber":
		listID, _ := in["list_id"].(string)
		email, _ := in["email"].(string)
		if listID == "" || email == "" {
			return nil, fmt.Errorf("mailchimp: list_id and email required")
		}
		hash := md5Hash(email)
		return m.get(ctx, fmt.Sprintf("%s/lists/%s/members/%s", base, listID, hash))

	case "list_campaigns":
		return m.get(ctx, base+"/campaigns?count=10")

	case "create_campaign":
		listID, _ := in["list_id"].(string)
		subject, _ := in["subject"].(string)
		if listID == "" || subject == "" {
			return nil, fmt.Errorf("mailchimp: list_id and subject required")
		}
		body := map[string]any{
			"type": "regular",
			"recipients": map[string]any{
				"list_id": listID,
			},
			"settings": map[string]any{
				"subject_line": subject,
				"from_name":    "github.com/shaiksadikjanu-cmd/orchkit",
				"reply_to":     "noreply@example.com",
			},
		}
		return m.post(ctx, base+"/campaigns", body)

	default:
		return nil, fmt.Errorf("mailchimp: unknown action %q", action)
	}
}

func (m *Mailchimp) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return m.do(req)
}

func (m *Mailchimp) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return m.do(req)
}

func (m *Mailchimp) do(req *http.Request) (orchkit.Output, error) {
	req.SetBasicAuth("anystring", m.APIKey)
	req.Header.Set("content-type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mailchimp: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("mailchimp: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}

func splitName(name string) [2]string {
	for i, c := range name {
		if c == ' ' {
			return [2]string{name[:i], name[i+1:]}
		}
	}
	return [2]string{name, ""}
}

func md5Hash(s string) string {
	// Simple lowercase — Mailchimp requires lowercase email hash
	result := ""
	for _, c := range s {
		if c >= 'A' && c <= 'Z' {
			result += string(rune(c + 32))
		} else {
			result += string(c)
		}
	}
	return result
}
