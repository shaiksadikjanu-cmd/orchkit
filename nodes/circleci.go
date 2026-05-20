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

// CircleCI interacts with the CircleCI API v2.
// Actions: list_pipelines, get_pipeline, trigger_pipeline,
//          list_workflows, get_workflow, list_jobs.
//
// Example:
//
//	nodes.NewCircleCI("your_api_token")
type CircleCI struct {
	Token  string
	client *http.Client
}

func NewCircleCI(token string) *CircleCI {
	return &CircleCI{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (c *CircleCI) Name() string { return "circleci" }

func (c *CircleCI) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with CircleCI. Actions: list_pipelines, get_pipeline, trigger_pipeline, list_workflows, get_workflow.",
		Params: map[string]any{
			"action":      map[string]any{"type": "string", "desc": "list_pipelines | get_pipeline | trigger_pipeline | list_workflows | get_workflow | list_jobs"},
			"project":     map[string]any{"type": "string", "desc": "Project slug e.g. 'gh/owner/repo'."},
			"pipeline_id": map[string]any{"type": "string", "desc": "Pipeline ID."},
			"workflow_id": map[string]any{"type": "string", "desc": "Workflow ID."},
			"branch":      map[string]any{"type": "string", "desc": "Branch name (trigger_pipeline)."},
		},
	}
}

func (c *CircleCI) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := "https://circleci.com/api/v2"
	action, _ := in["action"].(string)
	project, _ := in["project"].(string)

	switch action {
	case "list_pipelines", "":
		if project == "" {
			return nil, fmt.Errorf("circleci: project required")
		}
		return c.get(ctx, fmt.Sprintf("%s/project/%s/pipeline", base, project))

	case "get_pipeline":
		pipelineID, _ := in["pipeline_id"].(string)
		if pipelineID == "" {
			return nil, fmt.Errorf("circleci: pipeline_id required")
		}
		return c.get(ctx, base+"/pipeline/"+pipelineID)

	case "trigger_pipeline":
		if project == "" {
			return nil, fmt.Errorf("circleci: project required")
		}
		body := map[string]any{}
		if branch, ok := in["branch"].(string); ok {
			body["branch"] = branch
		}
		return c.post(ctx, fmt.Sprintf("%s/project/%s/pipeline", base, project), body)

	case "list_workflows":
		pipelineID, _ := in["pipeline_id"].(string)
		if pipelineID == "" {
			return nil, fmt.Errorf("circleci: pipeline_id required")
		}
		return c.get(ctx, base+"/pipeline/"+pipelineID+"/workflow")

	case "get_workflow":
		workflowID, _ := in["workflow_id"].(string)
		if workflowID == "" {
			return nil, fmt.Errorf("circleci: workflow_id required")
		}
		return c.get(ctx, base+"/workflow/"+workflowID)

	case "list_jobs":
		workflowID, _ := in["workflow_id"].(string)
		if workflowID == "" {
			return nil, fmt.Errorf("circleci: workflow_id required")
		}
		return c.get(ctx, base+"/workflow/"+workflowID+"/job")

	default:
		return nil, fmt.Errorf("circleci: unknown action %q", action)
	}
}

func (c *CircleCI) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *CircleCI) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *CircleCI) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("circle-token", c.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("circleci: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("circleci: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
