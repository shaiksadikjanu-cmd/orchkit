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

// HubSpot interacts with the HubSpot CRM API v3.
// Actions: create_contact, get_contact, update_contact,
//          list_contacts, create_deal, search.
//
// Example:
//
//	nodes.NewHubSpot("your_private_app_token")
type HubSpot struct {
	Token  string
	client *http.Client
}

func NewHubSpot(token string) *HubSpot {
	return &HubSpot{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (h *HubSpot) Name() string { return "hubspot" }

func (h *HubSpot) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with HubSpot CRM. Actions: create_contact, get_contact, update_contact, list_contacts, create_deal, search.",
		Params: map[string]any{
			"action":     map[string]any{"type": "string", "desc": "HubSpot action to perform."},
			"email":      map[string]any{"type": "string", "desc": "Contact email."},
			"first_name": map[string]any{"type": "string", "desc": "Contact first name."},
			"last_name":  map[string]any{"type": "string", "desc": "Contact last name."},
			"contact_id": map[string]any{"type": "string", "desc": "HubSpot contact ID."},
			"deal_name":  map[string]any{"type": "string", "desc": "Deal name (create_deal)."},
			"amount":     map[string]any{"type": "number", "desc": "Deal amount (create_deal)."},
			"query":      map[string]any{"type": "string", "desc": "Search query (search)."},
			"object":     map[string]any{"type": "string", "desc": "Object type to search: contacts|deals|companies. Default contacts."},
		},
	}
}

func (h *HubSpot) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	base := "https://api.hubapi.com/crm/v3"

	switch action {
	case "create_contact":
		props := map[string]any{}
		if v, ok := in["email"].(string); ok {
			props["email"] = v
		}
		if v, ok := in["first_name"].(string); ok {
			props["firstname"] = v
		}
		if v, ok := in["last_name"].(string); ok {
			props["lastname"] = v
		}
		return h.post(ctx, base+"/objects/contacts", map[string]any{"properties": props})

	case "get_contact":
		id, _ := in["contact_id"].(string)
		email, _ := in["email"].(string)
		if id != "" {
			return h.get(ctx, base+"/objects/contacts/"+id)
		}
		if email != "" {
			return h.get(ctx, base+"/objects/contacts/"+email+"?idProperty=email")
		}
		return nil, fmt.Errorf("hubspot: contact_id or email required")

	case "update_contact":
		id, _ := in["contact_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("hubspot: contact_id required for update")
		}
		props := map[string]any{}
		if v, ok := in["first_name"].(string); ok {
			props["firstname"] = v
		}
		if v, ok := in["last_name"].(string); ok {
			props["lastname"] = v
		}
		if v, ok := in["email"].(string); ok {
			props["email"] = v
		}
		return h.patch(ctx, base+"/objects/contacts/"+id, map[string]any{"properties": props})

	case "list_contacts", "":
		return h.get(ctx, base+"/objects/contacts?limit=50&properties=email,firstname,lastname")

	case "create_deal":
		name, _ := in["deal_name"].(string)
		if name == "" {
			return nil, fmt.Errorf("hubspot: deal_name required")
		}
		props := map[string]any{"dealname": name, "dealstage": "appointmentscheduled"}
		if v, ok := in["amount"].(float64); ok {
			props["amount"] = fmt.Sprintf("%.2f", v)
		}
		return h.post(ctx, base+"/objects/deals", map[string]any{"properties": props})

	case "search":
		query, _ := in["query"].(string)
		object, _ := in["object"].(string)
		if object == "" {
			object = "contacts"
		}
		body := map[string]any{
			"query": query,
			"limit": 10,
		}
		return h.post(ctx, base+"/objects/"+object+"/search", body)

	default:
		return nil, fmt.Errorf("hubspot: unknown action %q", action)
	}
}

func (h *HubSpot) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return h.do(req)
}

func (h *HubSpot) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return h.do(req)
}

func (h *HubSpot) patch(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return h.do(req)
}

func (h *HubSpot) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+h.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hubspot: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("hubspot: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
