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

// Todoist manages tasks via Todoist REST API v2.
// Actions: list_tasks, get_task, create_task, update_task,
//          close_task, delete_task, list_projects.
//
// Example:
//
//	nodes.NewTodoist("your_api_token")
type Todoist struct {
	Token  string
	client *http.Client
}

func NewTodoist(token string) *Todoist {
	return &Todoist{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (t *Todoist) Name() string { return "todoist" }

func (t *Todoist) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages Todoist tasks. Actions: list_tasks, get_task, create_task, update_task, close_task, delete_task, list_projects.",
		Params: map[string]any{
			"action":     map[string]any{"type": "string", "desc": "list_tasks | get_task | create_task | update_task | close_task | delete_task | list_projects"},
			"task_id":    map[string]any{"type": "string", "desc": "Task ID."},
			"project_id": map[string]any{"type": "string", "desc": "Project ID (list_tasks, create_task)."},
			"content":    map[string]any{"type": "string", "desc": "Task title/content (create_task, update_task)."},
			"due_string": map[string]any{"type": "string", "desc": "Natural language due date e.g. 'tomorrow', 'next Monday'."},
			"priority":   map[string]any{"type": "integer", "desc": "Priority 1(normal) to 4(urgent)."},
		},
	}
}

func (t *Todoist) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := "https://api.todoist.com/rest/v2"
	action, _ := in["action"].(string)

	switch action {
	case "list_projects":
		return t.get(ctx, base+"/projects")

	case "list_tasks", "":
		url := base + "/tasks"
		if pid, ok := in["project_id"].(string); ok && pid != "" {
			url += "?project_id=" + pid
		}
		return t.get(ctx, url)

	case "get_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("todoist: task_id required")
		}
		return t.get(ctx, base+"/tasks/"+taskID)

	case "create_task":
		content, _ := in["content"].(string)
		if content == "" {
			return nil, fmt.Errorf("todoist: content required")
		}
		body := map[string]any{"content": content}
		if pid, ok := in["project_id"].(string); ok {
			body["project_id"] = pid
		}
		if due, ok := in["due_string"].(string); ok {
			body["due_string"] = due
		}
		if p, ok := in["priority"].(float64); ok {
			body["priority"] = int(p)
		}
		return t.post(ctx, base+"/tasks", body)

	case "update_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("todoist: task_id required")
		}
		body := map[string]any{}
		if content, ok := in["content"].(string); ok {
			body["content"] = content
		}
		if due, ok := in["due_string"].(string); ok {
			body["due_string"] = due
		}
		if p, ok := in["priority"].(float64); ok {
			body["priority"] = int(p)
		}
		return t.post(ctx, base+"/tasks/"+taskID, body)

	case "close_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("todoist: task_id required")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/tasks/"+taskID+"/close", nil)
		if err != nil {
			return nil, err
		}
		return t.do(req)

	case "delete_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("todoist: task_id required")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/tasks/"+taskID, nil)
		if err != nil {
			return nil, err
		}
		return t.do(req)

	default:
		return nil, fmt.Errorf("todoist: unknown action %q", action)
	}
}

func (t *Todoist) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return t.do(req)
}

func (t *Todoist) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return t.do(req)
}

func (t *Todoist) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+t.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("todoist: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("todoist: api error %d: %s", resp.StatusCode, body)
	}
	if resp.StatusCode == 204 {
		return orchkit.Output{"success": true}, nil
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
