package nodes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"orchkit"
)

// Zendesk manages support tickets via Zendesk API v2.
// Actions: list_tickets, get_ticket, create_ticket,
//          update_ticket, add_comment, list_users.
//
// Example:
//
//	nodes.NewZendesk("your-subdomain", "email@example.com", "api_token")
type Zendesk struct {
	Subdomain string
	Email     string
	Token     string
	client    *http.Client
}

func NewZendesk(subdomain, email, token string) *Zendesk {
	return &Zendesk{Subdomain: subdomain, Email: email, Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (z *Zendesk) Name() string { return "zendesk" }

func (z *Zendesk) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages Zendesk support tickets. Actions: list_tickets, get_ticket, create_ticket, update_ticket, add_comment.",
		Params: map[string]any{
			"action":    map[string]any{"type": "string", "desc": "list_tickets | get_ticket | create_ticket | update_ticket | add_comment"},
			"ticket_id": map[string]any{"type": "string", "desc": "Ticket ID."},
			"subject":   map[string]any{"type": "string", "desc": "Ticket subject (create_ticket)."},
			"body":      map[string]any{"type": "string", "desc": "Ticket body or comment text."},
			"priority":  map[string]any{"type": "string", "desc": "Ticket priority: low | normal | high | urgent."},
			"status":    map[string]any{"type": "string", "desc": "Ticket status: open | pending | solved | closed."},
			"email":     map[string]any{"type": "string", "desc": "Requester email (create_ticket)."},
		},
	}
}

func (z *Zendesk) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := fmt.Sprintf("https://%s.zendesk.com/api/v2", z.Subdomain)
	action, _ := in["action"].(string)

	switch action {
	case "list_tickets", "":
		return z.get(ctx, base+"/tickets.json")

	case "get_ticket":
		id, _ := in["ticket_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("zendesk: ticket_id required")
		}
		return z.get(ctx, fmt.Sprintf("%s/tickets/%s.json", base, id))

	case "create_ticket":
		subject, _ := in["subject"].(string)
		body, _ := in["body"].(string)
		if subject == "" {
			return nil, fmt.Errorf("zendesk: subject required")
		}
		ticket := map[string]any{
			"subject": subject,
			"comment": map[string]any{"body": body},
		}
		if email, ok := in["email"].(string); ok {
			ticket["requester"] = map[string]any{"email": email}
		}
		if priority, ok := in["priority"].(string); ok {
			ticket["priority"] = priority
		}
		return z.post(ctx, base+"/tickets.json", map[string]any{"ticket": ticket})

	case "update_ticket":
		id, _ := in["ticket_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("zendesk: ticket_id required")
		}
		ticket := map[string]any{}
		if status, ok := in["status"].(string); ok {
			ticket["status"] = status
		}
		if priority, ok := in["priority"].(string); ok {
			ticket["priority"] = priority
		}
		return z.put(ctx, fmt.Sprintf("%s/tickets/%s.json", base, id), map[string]any{"ticket": ticket})

	case "add_comment":
		id, _ := in["ticket_id"].(string)
		body, _ := in["body"].(string)
		if id == "" || body == "" {
			return nil, fmt.Errorf("zendesk: ticket_id and body required")
		}
		ticket := map[string]any{"comment": map[string]any{"body": body}}
		return z.put(ctx, fmt.Sprintf("%s/tickets/%s.json", base, id), map[string]any{"ticket": ticket})

	default:
		return nil, fmt.Errorf("zendesk: unknown action %q", action)
	}
}

func (z *Zendesk) auth() string {
	creds := z.Email + "/token:" + z.Token
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
}

func (z *Zendesk) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return z.do(req)
}

func (z *Zendesk) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return z.do(req)
}

func (z *Zendesk) put(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return z.do(req)
}

func (z *Zendesk) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", z.auth())
	req.Header.Set("content-type", "application/json")
	resp, err := z.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zendesk: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("zendesk: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
