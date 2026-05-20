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

// GitHub interacts with the GitHub REST API.
// Supports: create issue, list issues, create comment, get repo info.
//
// Example:
//
//	nodes.NewGitHub("ghp_your_token")
type GitHub struct {
	Token  string
	client *http.Client
}

func NewGitHub(token string) *GitHub {
	return &GitHub{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (g *GitHub) Name() string { return "github" }

func (g *GitHub) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with GitHub API. Actions: create_issue, list_issues, create_comment, get_repo.",
		Params: map[string]any{
			"action": map[string]any{"type": "string", "desc": "Action: create_issue | list_issues | create_comment | get_repo"},
			"owner":  map[string]any{"type": "string", "desc": "Repository owner (username or org)."},
			"repo":   map[string]any{"type": "string", "desc": "Repository name."},
			"title":  map[string]any{"type": "string", "desc": "Issue title (create_issue)."},
			"body":   map[string]any{"type": "string", "desc": "Issue or comment body."},
			"number": map[string]any{"type": "integer", "desc": "Issue number (create_comment)."},
		},
	}
}

func (g *GitHub) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	owner, _ := in["owner"].(string)
	repo, _ := in["repo"].(string)

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("github: owner and repo are required")
	}

	base := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	switch action {
	case "create_issue":
		title, _ := in["title"].(string)
		body, _ := in["body"].(string)
		if title == "" {
			return nil, fmt.Errorf("github: title required for create_issue")
		}
		return g.post(ctx, base+"/issues", map[string]any{"title": title, "body": body})

	case "list_issues":
		return g.get(ctx, base+"/issues")

	case "create_comment":
		number, ok := in["number"].(float64)
		if !ok {
			return nil, fmt.Errorf("github: number required for create_comment")
		}
		body, _ := in["body"].(string)
		url := fmt.Sprintf("%s/issues/%d/comments", base, int(number))
		return g.post(ctx, url, map[string]any{"body": body})

	case "get_repo", "":
		return g.get(ctx, base)

	default:
		return nil, fmt.Errorf("github: unknown action %q", action)
	}
}

func (g *GitHub) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return g.do(req)
}

func (g *GitHub) post(ctx context.Context, url string, payload map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return g.do(req)
}

func (g *GitHub) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+g.Token)
	req.Header.Set("accept", "application/vnd.github+json")
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-github-api-version", "2022-11-28")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("github: api error %d: %s", resp.StatusCode, body)
	}

	var result any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("github: parse response: %w", err)
	}

	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
