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

// Asana manages tasks and projects via Asana API v1.
// Actions: list_tasks, get_task, create_task, update_task,
//          complete_task, list_projects.
//
// Example:
//
//	nodes.NewAsana("your_personal_access_token")
type Asana struct {
	Token  string
	client *http.Client
}

func NewAsana(token string) *Asana {
	return &Asana{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (a *Asana) Name() string { return "asana" }

func (a *Asana) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages Asana tasks and projects. Actions: list_tasks, get_task, create_task, update_task, complete_task, list_projects.",
		Params: map[string]any{
			"action":     map[string]any{"type": "string", "desc": "list_tasks | get_task | create_task | update_task | complete_task | list_projects"},
			"task_id":    map[string]any{"type": "string", "desc": "Task GID (get_task, update_task, complete_task)."},
			"project_id": map[string]any{"type": "string", "desc": "Project GID (list_tasks, create_task)."},
			"workspace":  map[string]any{"type": "string", "desc": "Workspace GID (list_projects)."},
			"name":       map[string]any{"type": "string", "desc": "Task name (create_task)."},
			"notes":      map[string]any{"type": "string", "desc": "Task description (create_task, update_task)."},
			"due_on":     map[string]any{"type": "string", "desc": "Due date YYYY-MM-DD (create_task, update_task)."},
		},
	}
}

func (a *Asana) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := "https://app.asana.com/api/1.0"
	action, _ := in["action"].(string)

	switch action {
	case "list_projects", "":
		workspace, _ := in["workspace"].(string)
		url := base + "/projects"
		if workspace != "" {
			url += "?workspace=" + workspace
		}
		return a.get(ctx, url)

	case "list_tasks":
		projectID, _ := in["project_id"].(string)
		if projectID == "" {
			return nil, fmt.Errorf("asana: project_id required for list_tasks")
		}
		return a.get(ctx, fmt.Sprintf("%s/projects/%s/tasks?opt_fields=name,completed,due_on,notes", base, projectID))

	case "get_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("asana: task_id required")
		}
		return a.get(ctx, base+"/tasks/"+taskID)

	case "create_task":
		name, _ := in["name"].(string)
		projectID, _ := in["project_id"].(string)
		if name == "" {
			return nil, fmt.Errorf("asana: name required")
		}
		data := map[string]any{"name": name}
		if projectID != "" {
			data["projects"] = []string{projectID}
		}
		if notes, ok := in["notes"].(string); ok {
			data["notes"] = notes
		}
		if due, ok := in["due_on"].(string); ok {
			data["due_on"] = due
		}
		return a.post(ctx, base+"/tasks", map[string]any{"data": data})

	case "update_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("asana: task_id required")
		}
		data := map[string]any{}
		if name, ok := in["name"].(string); ok {
			data["name"] = name
		}
		if notes, ok := in["notes"].(string); ok {
			data["notes"] = notes
		}
		if due, ok := in["due_on"].(string); ok {
			data["due_on"] = due
		}
		return a.put(ctx, base+"/tasks/"+taskID, map[string]any{"data": data})

	case "complete_task":
		taskID, _ := in["task_id"].(string)
		if taskID == "" {
			return nil, fmt.Errorf("asana: task_id required")
		}
		return a.put(ctx, base+"/tasks/"+taskID, map[string]any{"data": map[string]any{"completed": true}})

	default:
		return nil, fmt.Errorf("asana: unknown action %q", action)
	}
}

func (a *Asana) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return a.do(req)
}

func (a *Asana) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return a.do(req)
}

func (a *Asana) put(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return a.do(req)
}

func (a *Asana) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+a.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asana: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("asana: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
