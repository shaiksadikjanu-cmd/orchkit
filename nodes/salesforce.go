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

// Salesforce interacts with Salesforce via the REST API.
// Actions: query (SOQL), create, update, delete, get.
//
// Requires an access token (use OAuth2 or Connected App flow to obtain one).
//
// Example:
//
//	nodes.NewSalesforce("https://yourorg.salesforce.com", "access_token")
type Salesforce struct {
	InstanceURL string // e.g. "https://yourorg.salesforce.com"
	Token       string
	client      *http.Client
}

func NewSalesforce(instanceURL, token string) *Salesforce {
	return &Salesforce{
		InstanceURL: instanceURL,
		Token:       token,
		client:      &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Salesforce) Name() string { return "salesforce" }

func (s *Salesforce) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with Salesforce CRM. Actions: query (SOQL), create, update, delete, get.",
		Params: map[string]any{
			"action": map[string]any{"type": "string", "desc": "query | create | update | delete | get"},
			"soql":   map[string]any{"type": "string", "desc": "SOQL query string (query action)."},
			"object": map[string]any{"type": "string", "desc": "Salesforce object type e.g. Contact, Lead, Account."},
			"id":     map[string]any{"type": "string", "desc": "Record ID (get, update, delete)."},
			"fields": map[string]any{"type": "object", "desc": "Field values map (create, update)."},
		},
	}
}

func (s *Salesforce) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	base := s.InstanceURL + "/services/data/v58.0"

	switch action {
	case "query", "":
		soql, _ := in["soql"].(string)
		if soql == "" {
			return nil, fmt.Errorf("salesforce: soql required for query")
		}
		return s.get(ctx, base+"/query?q="+soql)

	case "create":
		obj, _ := in["object"].(string)
		fields, _ := in["fields"].(map[string]any)
		if obj == "" || fields == nil {
			return nil, fmt.Errorf("salesforce: object and fields required for create")
		}
		return s.post(ctx, base+"/sobjects/"+obj, fields)

	case "update":
		obj, _ := in["object"].(string)
		id, _ := in["id"].(string)
		fields, _ := in["fields"].(map[string]any)
		if obj == "" || id == "" || fields == nil {
			return nil, fmt.Errorf("salesforce: object, id, and fields required for update")
		}
		return s.patch(ctx, base+"/sobjects/"+obj+"/"+id, fields)

	case "delete":
		obj, _ := in["object"].(string)
		id, _ := in["id"].(string)
		if obj == "" || id == "" {
			return nil, fmt.Errorf("salesforce: object and id required for delete")
		}
		return s.del(ctx, base+"/sobjects/"+obj+"/"+id)

	case "get":
		obj, _ := in["object"].(string)
		id, _ := in["id"].(string)
		if obj == "" || id == "" {
			return nil, fmt.Errorf("salesforce: object and id required for get")
		}
		return s.get(ctx, base+"/sobjects/"+obj+"/"+id)

	default:
		return nil, fmt.Errorf("salesforce: unknown action %q", action)
	}
}

func (s *Salesforce) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return s.do(req)
}

func (s *Salesforce) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return s.do(req)
}

func (s *Salesforce) patch(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return s.do(req)
}

func (s *Salesforce) del(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	return s.do(req)
}

func (s *Salesforce) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+s.Token)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("salesforce: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("salesforce: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	if len(body) > 0 {
		json.Unmarshal(body, &result)
	}
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
