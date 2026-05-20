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

// GitLab interacts with the GitLab REST API v4.
// Actions: list_issues, get_issue, create_issue, create_mr,
//          list_mrs, get_pipeline, list_pipelines.
//
// Example:
//
//	nodes.NewGitLab("glpat_your_token", "gitlab.com")
type GitLab struct {
	Token  string
	Host   string // defaults to "gitlab.com"
	client *http.Client
}

func NewGitLab(token, host string) *GitLab {
	if host == "" {
		host = "gitlab.com"
	}
	return &GitLab{Token: token, Host: host, client: &http.Client{Timeout: 15 * time.Second}}
}

func (g *GitLab) Name() string { return "gitlab" }

func (g *GitLab) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with GitLab API. Actions: list_issues, get_issue, create_issue, create_mr, list_pipelines.",
		Params: map[string]any{
			"action":     map[string]any{"type": "string", "desc": "list_issues | get_issue | create_issue | create_mr | list_pipelines | get_pipeline"},
			"project":    map[string]any{"type": "string", "desc": "Project path e.g. 'namespace/project' or numeric ID."},
			"issue_id":   map[string]any{"type": "string", "desc": "Issue IID (get_issue)."},
			"title":      map[string]any{"type": "string", "desc": "Issue or MR title."},
			"desc":       map[string]any{"type": "string", "desc": "Issue or MR description."},
			"source":     map[string]any{"type": "string", "desc": "Source branch (create_mr)."},
			"target":     map[string]any{"type": "string", "desc": "Target branch (create_mr). Default main."},
			"pipeline_id": map[string]any{"type": "string", "desc": "Pipeline ID (get_pipeline)."},
		},
	}
}

func (g *GitLab) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	project, _ := in["project"].(string)
	if project == "" {
		return nil, fmt.Errorf("gitlab: project is required")
	}
	base := fmt.Sprintf("https://%s/api/v4/projects/%s", g.Host, project)
	action, _ := in["action"].(string)

	switch action {
	case "list_issues", "":
		return g.get(ctx, base+"/issues?state=opened")

	case "get_issue":
		issueID, _ := in["issue_id"].(string)
		if issueID == "" {
			return nil, fmt.Errorf("gitlab: issue_id required")
		}
		return g.get(ctx, base+"/issues/"+issueID)

	case "create_issue":
		title, _ := in["title"].(string)
		if title == "" {
			return nil, fmt.Errorf("gitlab: title required")
		}
		body := map[string]any{"title": title}
		if desc, ok := in["desc"].(string); ok {
			body["description"] = desc
		}
		return g.post(ctx, base+"/issues", body)

	case "create_mr":
		title, _ := in["title"].(string)
		source, _ := in["source"].(string)
		target, _ := in["target"].(string)
		if title == "" || source == "" {
			return nil, fmt.Errorf("gitlab: title and source required for create_mr")
		}
		if target == "" {
			target = "main"
		}
		body := map[string]any{
			"title":         title,
			"source_branch": source,
			"target_branch": target,
		}
		if desc, ok := in["desc"].(string); ok {
			body["description"] = desc
		}
		return g.post(ctx, base+"/merge_requests", body)

	case "list_pipelines":
		return g.get(ctx, base+"/pipelines?per_page=10")

	case "get_pipeline":
		pipelineID, _ := in["pipeline_id"].(string)
		if pipelineID == "" {
			return nil, fmt.Errorf("gitlab: pipeline_id required")
		}
		return g.get(ctx, base+"/pipelines/"+pipelineID)

	default:
		return nil, fmt.Errorf("gitlab: unknown action %q", action)
	}
}

func (g *GitLab) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return g.do(req)
}

func (g *GitLab) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return g.do(req)
}

func (g *GitLab) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("private-token", g.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("gitlab: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
