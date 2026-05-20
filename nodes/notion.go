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

// Notion interacts with the Notion API.
// Actions: create_page, get_page, query_database, append_block.
//
// Example:
//
//	nodes.NewNotion("secret_your_integration_token")
type Notion struct {
	Token  string
	client *http.Client
}

func NewNotion(token string) *Notion {
	return &Notion{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (n *Notion) Name() string { return "notion" }

func (n *Notion) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with Notion API. Actions: create_page, get_page, query_database, append_block.",
		Params: map[string]any{
			"action":      map[string]any{"type": "string", "desc": "create_page | get_page | query_database | append_block"},
			"database_id": map[string]any{"type": "string", "desc": "Notion database ID."},
			"page_id":     map[string]any{"type": "string", "desc": "Notion page ID."},
			"title":       map[string]any{"type": "string", "desc": "Page title (create_page)."},
			"properties":  map[string]any{"type": "object", "desc": "Page properties map (create_page)."},
			"filter":      map[string]any{"type": "object", "desc": "Database filter (query_database)."},
			"content":     map[string]any{"type": "string", "desc": "Text content to append (append_block)."},
		},
	}
}

func (n *Notion) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	if action == "" {
		action = "get_page"
	}

	switch action {
	case "create_page":
		return n.createPage(ctx, in)
	case "get_page":
		pageID, _ := in["page_id"].(string)
		if pageID == "" {
			return nil, fmt.Errorf("notion: page_id required for get_page")
		}
		return n.get(ctx, "/pages/"+pageID)
	case "query_database":
		dbID, _ := in["database_id"].(string)
		if dbID == "" {
			return nil, fmt.Errorf("notion: database_id required for query_database")
		}
		body := map[string]any{}
		if f, ok := in["filter"].(map[string]any); ok {
			body["filter"] = f
		}
		return n.post(ctx, "/databases/"+dbID+"/query", body)
	case "append_block":
		pageID, _ := in["page_id"].(string)
		content, _ := in["content"].(string)
		if pageID == "" || content == "" {
			return nil, fmt.Errorf("notion: page_id and content required for append_block")
		}
		body := map[string]any{
			"children": []any{
				map[string]any{
					"object": "block",
					"type":   "paragraph",
					"paragraph": map[string]any{
						"rich_text": []any{
							map[string]any{
								"type": "text",
								"text": map[string]any{"content": content},
							},
						},
					},
				},
			},
		}
		return n.patch(ctx, "/blocks/"+pageID+"/children", body)
	default:
		return nil, fmt.Errorf("notion: unknown action %q", action)
	}
}

func (n *Notion) createPage(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	dbID, _ := in["database_id"].(string)
	title, _ := in["title"].(string)
	if dbID == "" {
		return nil, fmt.Errorf("notion: database_id required for create_page")
	}

	props, _ := in["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	if title != "" {
		props["Name"] = map[string]any{
			"title": []any{
				map[string]any{"text": map[string]any{"content": title}},
			},
		}
	}

	body := map[string]any{
		"parent":     map[string]any{"database_id": dbID},
		"properties": props,
	}
	return n.post(ctx, "/pages", body)
}

func (n *Notion) get(ctx context.Context, path string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.notion.com/v1"+path, nil)
	if err != nil {
		return nil, err
	}
	return n.do(req)
}

func (n *Notion) post(ctx context.Context, path string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.notion.com/v1"+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return n.do(req)
}

func (n *Notion) patch(ctx context.Context, path string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, "https://api.notion.com/v1"+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return n.do(req)
}

func (n *Notion) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+n.Token)
	req.Header.Set("notion-version", "2022-06-28")
	req.Header.Set("content-type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("notion: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("notion: api error %d: %s", resp.StatusCode, body)
	}

	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
