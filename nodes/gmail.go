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

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Gmail sends and reads emails via the Gmail API.
// Requires an OAuth2 access token with gmail.send or gmail.readonly scope.
// Actions: send, list, get.
//
// Example:
//
//	nodes.NewGmail("ya29.access_token")
type Gmail struct {
	Token  string
	client *http.Client
}

func NewGmail(token string) *Gmail {
	return &Gmail{Token: token, client: &http.Client{Timeout: 30 * time.Second}}
}

func (g *Gmail) Name() string { return "gmail" }

func (g *Gmail) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends and reads Gmail messages via Gmail API. Actions: send, list, get.",
		Params: map[string]any{
			"action":  map[string]any{"type": "string", "desc": "send | list | get"},
			"to":      map[string]any{"type": "string", "desc": "Recipient email (send)."},
			"subject": map[string]any{"type": "string", "desc": "Email subject (send)."},
			"body":    map[string]any{"type": "string", "desc": "Email body text (send)."},
			"id":      map[string]any{"type": "string", "desc": "Message ID (get)."},
			"query":   map[string]any{"type": "string", "desc": "Gmail search query (list) e.g. 'is:unread'."},
			"limit":   map[string]any{"type": "number", "desc": "Max messages to list. Default 10."},
		},
	}
}

func (g *Gmail) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	if action == "" {
		action = "list"
	}

	base := "https://gmail.googleapis.com/gmail/v1/users/me"

	switch action {
	case "send":
		to, _ := in["to"].(string)
		subject, _ := in["subject"].(string)
		body, _ := in["body"].(string)
		if to == "" {
			return nil, fmt.Errorf("gmail: 'to' required for send")
		}
		raw := fmt.Sprintf("To: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
			to, subject, body)
		encoded := base64.URLEncoding.EncodeToString([]byte(raw))
		return g.post(ctx, base+"/messages/send", map[string]any{"raw": encoded})

	case "list":
		query, _ := in["query"].(string)
		limit := 10
		if v, ok := in["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		url := fmt.Sprintf("%s/messages?maxResults=%d", base, limit)
		if query != "" {
			url += "&q=" + query
		}
		return g.get(ctx, url)

	case "get":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("gmail: message id required for get")
		}
		return g.get(ctx, base+"/messages/"+id)

	default:
		return nil, fmt.Errorf("gmail: unknown action %q", action)
	}
}

func (g *Gmail) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return g.do(req)
}

func (g *Gmail) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return g.do(req)
}

func (g *Gmail) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+g.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gmail: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gmail: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
