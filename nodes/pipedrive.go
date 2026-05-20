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

// Pipedrive manages deals and contacts via Pipedrive CRM API v1.
// Actions: list_deals, get_deal, create_deal, update_deal,
//          list_persons, create_person, search.
//
// Example:
//
//	nodes.NewPipedrive("your_api_token")
type Pipedrive struct {
	Token  string
	client *http.Client
}

func NewPipedrive(token string) *Pipedrive {
	return &Pipedrive{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (p *Pipedrive) Name() string { return "pipedrive" }

func (p *Pipedrive) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages Pipedrive CRM deals and contacts. Actions: list_deals, get_deal, create_deal, update_deal, list_persons, create_person.",
		Params: map[string]any{
			"action":    map[string]any{"type": "string", "desc": "list_deals | get_deal | create_deal | update_deal | list_persons | create_person | search"},
			"deal_id":   map[string]any{"type": "string", "desc": "Deal ID."},
			"person_id": map[string]any{"type": "string", "desc": "Person ID."},
			"title":     map[string]any{"type": "string", "desc": "Deal title (create_deal)."},
			"name":      map[string]any{"type": "string", "desc": "Person name (create_person)."},
			"email":     map[string]any{"type": "string", "desc": "Person email (create_person)."},
			"value":     map[string]any{"type": "number", "desc": "Deal value (create_deal, update_deal)."},
			"status":    map[string]any{"type": "string", "desc": "Deal status: open | won | lost (update_deal)."},
			"query":     map[string]any{"type": "string", "desc": "Search term (search action)."},
		},
	}
}

func (p *Pipedrive) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := "https://api.pipedrive.com/v1"
	auth := "?api_token=" + p.Token
	action, _ := in["action"].(string)

	switch action {
	case "list_deals", "":
		return p.get(ctx, base+"/deals"+auth)

	case "get_deal":
		id, _ := in["deal_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("pipedrive: deal_id required")
		}
		return p.get(ctx, base+"/deals/"+id+auth)

	case "create_deal":
		title, _ := in["title"].(string)
		if title == "" {
			return nil, fmt.Errorf("pipedrive: title required")
		}
		body := map[string]any{"title": title}
		if value, ok := in["value"].(float64); ok {
			body["value"] = value
		}
		if personID, ok := in["person_id"].(string); ok {
			body["person_id"] = personID
		}
		return p.post(ctx, base+"/deals"+auth, body)

	case "update_deal":
		id, _ := in["deal_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("pipedrive: deal_id required")
		}
		body := map[string]any{}
		if status, ok := in["status"].(string); ok {
			body["status"] = status
		}
		if value, ok := in["value"].(float64); ok {
			body["value"] = value
		}
		if title, ok := in["title"].(string); ok {
			body["title"] = title
		}
		return p.put(ctx, base+"/deals/"+id+auth, body)

	case "list_persons":
		return p.get(ctx, base+"/persons"+auth)

	case "create_person":
		name, _ := in["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("pipedrive: name required")
		}
		body := map[string]any{"name": name}
		if email, ok := in["email"].(string); ok {
			body["email"] = []map[string]any{{"value": email, "primary": true}}
		}
		return p.post(ctx, base+"/persons"+auth, body)

	case "search":
		query, _ := in["query"].(string)
		if query == "" {
			return nil, fmt.Errorf("pipedrive: query required")
		}
		return p.get(ctx, fmt.Sprintf("%s/itemSearch%s&term=%s", base, auth, query))

	default:
		return nil, fmt.Errorf("pipedrive: unknown action %q", action)
	}
}

func (p *Pipedrive) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return p.do(req)
}

func (p *Pipedrive) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return p.do(req)
}

func (p *Pipedrive) put(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return p.do(req)
}

func (p *Pipedrive) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("content-type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pipedrive: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("pipedrive: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
