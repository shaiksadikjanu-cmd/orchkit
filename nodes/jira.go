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

// Jira interacts with Jira Cloud via the REST API v3.
// Actions: get_issue, create_issue, update_issue, list_issues, add_comment, transition.
//
// Example:
//
//	nodes.NewJira("your-domain.atlassian.net", "email@example.com", "api_token")
type Jira struct {
	Domain string // e.g. "your-domain.atlassian.net"
	Email  string
	Token  string
	client *http.Client
}

func NewJira(domain, email, token string) *Jira {
	return &Jira{Domain: domain, Email: email, Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (j *Jira) Name() string { return "jira" }

func (j *Jira) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with Jira Cloud. Actions: get_issue, create_issue, update_issue, list_issues, add_comment, transition.",
		Params: map[string]any{
			"action":      map[string]any{"type": "string", "desc": "get_issue | create_issue | update_issue | list_issues | add_comment | transition"},
			"issue_key":   map[string]any{"type": "string", "desc": "Issue key e.g. PROJ-123."},
			"project_key": map[string]any{"type": "string", "desc": "Project key e.g. PROJ (create_issue)."},
			"summary":     map[string]any{"type": "string", "desc": "Issue summary/title."},
			"description": map[string]any{"type": "string", "desc": "Issue description."},
			"issue_type":  map[string]any{"type": "string", "desc": "Issue type e.g. Bug, Story, Task."},
			"comment":     map[string]any{"type": "string", "desc": "Comment text (add_comment)."},
			"jql":         map[string]any{"type": "string", "desc": "JQL query (list_issues)."},
			"status":      map[string]any{"type": "string", "desc": "Target status name (transition)."},
		},
	}
}

func (j *Jira) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	base := fmt.Sprintf("https://%s/rest/api/3", j.Domain)

	switch action {
	case "get_issue", "":
		key, _ := in["issue_key"].(string)
		if key == "" {
			return nil, fmt.Errorf("jira: issue_key required")
		}
		return j.get(ctx, base+"/issue/"+key)

	case "create_issue":
		summary, _ := in["summary"].(string)
		projectKey, _ := in["project_key"].(string)
		if summary == "" || projectKey == "" {
			return nil, fmt.Errorf("jira: summary and project_key required")
		}
		issueType, _ := in["issue_type"].(string)
		if issueType == "" {
			issueType = "Task"
		}
		desc, _ := in["description"].(string)
		body := map[string]any{
			"fields": map[string]any{
				"project":   map[string]any{"key": projectKey},
				"summary":   summary,
				"issuetype": map[string]any{"name": issueType},
				"description": map[string]any{
					"type":    "doc",
					"version": 1,
					"content": []any{
						map[string]any{
							"type": "paragraph",
							"content": []any{
								map[string]any{"type": "text", "text": desc},
							},
						},
					},
				},
			},
		}
		return j.post(ctx, base+"/issue", body)

	case "update_issue":
		key, _ := in["issue_key"].(string)
		if key == "" {
			return nil, fmt.Errorf("jira: issue_key required")
		}
		fields := map[string]any{}
		if s, ok := in["summary"].(string); ok && s != "" {
			fields["summary"] = s
		}
		return j.put(ctx, base+"/issue/"+key, map[string]any{"fields": fields})

	case "list_issues":
		jql, _ := in["jql"].(string)
		if jql == "" {
			jql = "order by created DESC"
		}
		return j.post(ctx, base+"/search", map[string]any{"jql": jql, "maxResults": 50})

	case "add_comment":
		key, _ := in["issue_key"].(string)
		comment, _ := in["comment"].(string)
		if key == "" || comment == "" {
			return nil, fmt.Errorf("jira: issue_key and comment required")
		}
		body := map[string]any{
			"body": map[string]any{
				"type":    "doc",
				"version": 1,
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{"type": "text", "text": comment},
						},
					},
				},
			},
		}
		return j.post(ctx, base+"/issue/"+key+"/comment", body)

	case "transition":
		key, _ := in["issue_key"].(string)
		status, _ := in["status"].(string)
		if key == "" || status == "" {
			return nil, fmt.Errorf("jira: issue_key and status required")
		}
		// Get available transitions.
		out, err := j.get(ctx, base+"/issue/"+key+"/transitions")
		if err != nil {
			return nil, err
		}
		result, _ := out["result"].(map[string]any)
		transitions, _ := result["transitions"].([]any)
		var transitionID string
		for _, t := range transitions {
			if tm, ok := t.(map[string]any); ok {
				if nm, ok := tm["name"].(map[string]any); ok {
					_ = nm
				}
				if fmt.Sprint(tm["name"]) == status {
					transitionID = fmt.Sprint(tm["id"])
					break
				}
			}
		}
		if transitionID == "" {
			return nil, fmt.Errorf("jira: transition %q not found", status)
		}
		return j.post(ctx, base+"/issue/"+key+"/transitions",
			map[string]any{"transition": map[string]any{"id": transitionID}})

	default:
		return nil, fmt.Errorf("jira: unknown action %q", action)
	}
}

func (j *Jira) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return j.do(req)
}

func (j *Jira) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return j.do(req)
}

func (j *Jira) put(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return j.do(req)
}

func (j *Jira) do(req *http.Request) (orchkit.Output, error) {
	creds := base64.StdEncoding.EncodeToString([]byte(j.Email + ":" + j.Token))
	req.Header.Set("authorization", "Basic "+creds)
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jira: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("jira: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
