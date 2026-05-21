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

// WordPress interacts with WordPress via the REST API v2.
// Actions: list_posts, get_post, create_post, update_post, delete_post.
//
// Example:
//
//	nodes.NewWordPress("https://your-site.com", "username", "app_password")
type WordPress struct {
	BaseURL  string
	Username string
	Password string // Application Password from WordPress admin
	client   *http.Client
}

func NewWordPress(baseURL, username, password string) *WordPress {
	return &WordPress{BaseURL: baseURL, Username: username, Password: password, client: &http.Client{Timeout: 15 * time.Second}}
}

func (w *WordPress) Name() string { return "wordpress" }

func (w *WordPress) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages WordPress posts via REST API. Actions: list_posts, get_post, create_post, update_post, delete_post.",
		Params: map[string]any{
			"action":  map[string]any{"type": "string", "desc": "list_posts | get_post | create_post | update_post | delete_post"},
			"id":      map[string]any{"type": "string", "desc": "Post ID (get, update, delete)."},
			"title":   map[string]any{"type": "string", "desc": "Post title (create, update)."},
			"content": map[string]any{"type": "string", "desc": "Post content HTML (create, update)."},
			"status":  map[string]any{"type": "string", "desc": "Post status: draft | publish | private. Default draft."},
			"limit":   map[string]any{"type": "number", "desc": "Max posts to list. Default 10."},
		},
	}
}

func (w *WordPress) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := w.BaseURL + "/wp-json/wp/v2"
	action, _ := in["action"].(string)

	switch action {
	case "list_posts", "":
		limit := 10
		if v, ok := in["limit"].(float64); ok {
			limit = int(v)
		}
		return w.get(ctx, fmt.Sprintf("%s/posts?per_page=%d", base, limit))

	case "get_post":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("wordpress: id required")
		}
		return w.get(ctx, base+"/posts/"+id)

	case "create_post":
		title, _ := in["title"].(string)
		if title == "" {
			return nil, fmt.Errorf("wordpress: title required")
		}
		status, _ := in["status"].(string)
		if status == "" {
			status = "draft"
		}
		content, _ := in["content"].(string)
		body := map[string]any{
			"title":   title,
			"content": content,
			"status":  status,
		}
		return w.post(ctx, base+"/posts", body)

	case "update_post":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("wordpress: id required")
		}
		body := map[string]any{}
		if t, ok := in["title"].(string); ok {
			body["title"] = t
		}
		if c, ok := in["content"].(string); ok {
			body["content"] = c
		}
		if s, ok := in["status"].(string); ok {
			body["status"] = s
		}
		return w.patch(ctx, base+"/posts/"+id, body)

	case "delete_post":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("wordpress: id required")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
			base+"/posts/"+id+"?force=true", nil)
		if err != nil {
			return nil, err
		}
		return w.do(req)

	default:
		return nil, fmt.Errorf("wordpress: unknown action %q", action)
	}
}

func (w *WordPress) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return w.do(req)
}

func (w *WordPress) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return w.do(req)
}

func (w *WordPress) patch(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return w.do(req)
}

func (w *WordPress) do(req *http.Request) (orchkit.Output, error) {
	req.SetBasicAuth(w.Username, w.Password)
	req.Header.Set("content-type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wordpress: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("wordpress: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
