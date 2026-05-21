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

// Airtable interacts with Airtable via the REST API.
// Actions: list, get, create, update, delete.
//
// Example:
//
//	nodes.NewAirtable("pat_your_token", "app_base_id", "TableName")
type Airtable struct {
	Token   string
	BaseID  string
	Table   string
	client  *http.Client
}

func NewAirtable(token, baseID, table string) *Airtable {
	return &Airtable{
		Token:  token,
		BaseID: baseID,
		Table:  table,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (a *Airtable) Name() string { return "airtable" }

func (a *Airtable) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Reads and writes Airtable records. Actions: list, get, create, update, delete.",
		Params: map[string]any{
			"action":    map[string]any{"type": "string", "desc": "list | get | create | update | delete"},
			"record_id": map[string]any{"type": "string", "desc": "Airtable record ID (get, update, delete)."},
			"fields":    map[string]any{"type": "object", "desc": "Field values map (create, update)."},
			"filter":    map[string]any{"type": "string", "desc": "Airtable formula filter (list) e.g. {Status}='Active'."},
			"limit":     map[string]any{"type": "number", "desc": "Max records to return (list). Default 100."},
			"table":     map[string]any{"type": "string", "desc": "Table name. Falls back to constructor."},
		},
	}
}

func (a *Airtable) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	table := a.Table
	if v, ok := in["table"].(string); ok && v != "" {
		table = v
	}
	if table == "" {
		return nil, fmt.Errorf("airtable: table name required")
	}

	base := fmt.Sprintf("https://api.airtable.com/v0/%s/%s", a.BaseID, table)
	action, _ := in["action"].(string)
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		url := base + "?pageSize=100"
		if limit, ok := in["limit"].(float64); ok && limit > 0 {
			url = fmt.Sprintf("%s?pageSize=%d", base, int(limit))
		}
		if filter, ok := in["filter"].(string); ok && filter != "" {
			url += "&filterByFormula=" + filter
		}
		return a.get(ctx, url)

	case "get":
		id, _ := in["record_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("airtable: record_id required for get")
		}
		return a.get(ctx, base+"/"+id)

	case "create":
		fields, _ := in["fields"].(map[string]any)
		if fields == nil {
			return nil, fmt.Errorf("airtable: fields required for create")
		}
		return a.post(ctx, base, map[string]any{
			"records": []any{map[string]any{"fields": fields}},
		})

	case "update":
		id, _ := in["record_id"].(string)
		fields, _ := in["fields"].(map[string]any)
		if id == "" || fields == nil {
			return nil, fmt.Errorf("airtable: record_id and fields required for update")
		}
		return a.patch(ctx, base+"/"+id, map[string]any{"fields": fields})

	case "delete":
		id, _ := in["record_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("airtable: record_id required for delete")
		}
		return a.del(ctx, base+"/"+id)

	default:
		return nil, fmt.Errorf("airtable: unknown action %q", action)
	}
}

func (a *Airtable) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return a.do(req)
}

func (a *Airtable) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return a.do(req)
}

func (a *Airtable) patch(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return a.do(req)
}

func (a *Airtable) del(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	return a.do(req)
}

func (a *Airtable) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+a.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("airtable: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("airtable: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
