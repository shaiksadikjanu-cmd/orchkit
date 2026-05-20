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

// ClickUp manages tasks via ClickUp API v2.
// Actions: list_tasks, get_task, create_task, update_task,
//          delete_task, list_spaces.
//
// Example:
//
//	nodes.NewClickUp("pk_your_api_key")
type ClickUp struct {
	Token  string
	client *http.Client
}

func NewClickUp(token string) *ClickUp {
	return &ClickUp{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (c *ClickUp) Name() string { return "clickup" }

func (c *ClickUp) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages ClickUp tasks and spaces. Actions: list_tasks, get_task, create_task, update_task, delete_task, list_spaces.",
		Params: map[string]any{
			"action":   map[string]any{"type": "string", "desc": "list_tasks | get_task | create_task | update_task | delete_task | list_spaces"},
			"task_id":  map[string]any{"type": "string", "desc": "Task ID."},
			"list_id":  map[string]any{"type": "string", "desc": "List ID (list_tasks, create_task)."},
			"team_id":  map[string]any{"type": "string", "desc": "Team/Workspace ID (list_spaces)."},
			"name":     map[string]any{"type": "string", "desc": "Task name."},
			"desc":     map[string]any{"type": "string", "desc": "Task description."},
			"status":   map[string]any{"type": "string", "desc": "Task status e.g. 'in progress', 'complete'."},
			"priority": map[string]any{"type": "integer", "desc": "Priority 1(urgent) to 4(low)."},
		},
	}
}

func (c *ClickUp) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := "https://api.clickup.com/api/v2"
	action, _ := in["action"].(string)

	switch action {
	case "list_spaces":
		teamID, _ := in["team_id"].(string)
		if teamID == "" {
			return nil, fmt.Errorf("clickup: team_id required")
		}
		return c.get(ctx, fmt.Sprintf("%s/team/%s/space", base, teamID))

	case "list_tasks", "":
		listID, _ := in["list_id"].(string)
		if listID == "" {
			return nil, fmt.Errorf("clickup: list_id required")
		}
		return c.get(ctx, fmt.Sprintf("%s/list/%s/task", base, listID))

	case "get_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("clickup: task_id required")
		}
		return c.get(ctx, base+"/task/"+taskID)

	case "create_task":
		listID, _ := in["list_id"].(string)
		name, _ := in["name"].(string)
		if listID == "" || name == "" {
			return nil, fmt.Errorf("clickup: list_id and name required")
		}
		body := map[string]any{"name": name}
		if desc, ok := in["desc"].(string); ok {
			body["description"] = desc
		}
		if status, ok := in["status"].(string); ok {
			body["status"] = status
		}
		if p, ok := in["priority"].(float64); ok {
			body["priority"] = int(p)
		}
		return c.post(ctx, fmt.Sprintf("%s/list/%s/task", base, listID), body)

	case "update_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("clickup: task_id required")
		}
		body := map[string]any{}
		if name, ok := in["name"].(string); ok {
			body["name"] = name
		}
		if status, ok := in["status"].(string); ok {
			body["status"] = status
		}
		if desc, ok := in["desc"].(string); ok {
			body["description"] = desc
		}
		return c.put(ctx, base+"/task/"+taskID, body)

	case "delete_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("clickup: task_id required")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/task/"+taskID, nil)
		if err != nil {
			return nil, err
		}
		return c.do(req)

	default:
		return nil, fmt.Errorf("clickup: unknown action %q", action)
	}
}

func (c *ClickUp) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *ClickUp) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *ClickUp) put(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *ClickUp) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", c.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clickup: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("clickup: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
