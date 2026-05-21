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

// Linear interacts with Linear issue tracker via GraphQL API.
// Actions: create_issue, get_issue, list_issues, update_issue, list_teams.
//
// Example:
//
//	nodes.NewLinear("lin_api_your_key")
type Linear struct {
	APIKey string
	client *http.Client
}

func NewLinear(apiKey string) *Linear {
	return &Linear{APIKey: apiKey, client: &http.Client{Timeout: 15 * time.Second}}
}

func (l *Linear) Name() string { return "linear" }

func (l *Linear) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with Linear issue tracker via GraphQL. Actions: create_issue, get_issue, list_issues, update_issue, list_teams.",
		Params: map[string]any{
			"action":      map[string]any{"type": "string", "desc": "create_issue | get_issue | list_issues | update_issue | list_teams"},
			"title":       map[string]any{"type": "string", "desc": "Issue title (create_issue)."},
			"description": map[string]any{"type": "string", "desc": "Issue description."},
			"team_id":     map[string]any{"type": "string", "desc": "Team ID (create_issue)."},
			"issue_id":    map[string]any{"type": "string", "desc": "Issue ID (get_issue, update_issue)."},
			"priority":    map[string]any{"type": "number", "desc": "Priority 0-4 (0=no, 1=urgent, 2=high, 3=medium, 4=low)."},
		},
	}
}

func (l *Linear) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)

	switch action {
	case "list_teams", "":
		return l.query(ctx, `{ teams { nodes { id name key } } }`, nil)

	case "list_issues":
		return l.query(ctx, `{ issues(first: 50) { nodes { id title priority state { name } team { name } } } }`, nil)

	case "get_issue":
		id, _ := in["issue_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("linear: issue_id required")
		}
		return l.query(ctx, `query($id:String!){issue(id:$id){id title description priority state{name}}}`,
			map[string]any{"id": id})

	case "create_issue":
		title, _ := in["title"].(string)
		teamID, _ := in["team_id"].(string)
		if title == "" || teamID == "" {
			return nil, fmt.Errorf("linear: title and team_id required")
		}
		vars := map[string]any{"title": title, "teamId": teamID}
		if desc, ok := in["description"].(string); ok {
			vars["description"] = desc
		}
		if p, ok := in["priority"].(float64); ok {
			vars["priority"] = int(p)
		}
		return l.query(ctx,
			`mutation($title:String!,$teamId:String!,$description:String,$priority:Int){
				issueCreate(input:{title:$title,teamId:$teamId,description:$description,priority:$priority}){
					issue{id title url}
				}
			}`, vars)

	case "update_issue":
		id, _ := in["issue_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("linear: issue_id required")
		}
		input := map[string]any{}
		if t, ok := in["title"].(string); ok {
			input["title"] = t
		}
		if p, ok := in["priority"].(float64); ok {
			input["priority"] = int(p)
		}
		return l.query(ctx,
			`mutation($id:String!,$input:IssueUpdateInput!){issueUpdate(id:$id,input:$input){issue{id title}}}`,
			map[string]any{"id": id, "input": input})

	default:
		return nil, fmt.Errorf("linear: unknown action %q", action)
	}
}

func (l *Linear) query(ctx context.Context, query string, variables map[string]any) (orchkit.Output, error) {
	body := map[string]any{"query": query}
	if variables != nil {
		body["variables"] = variables
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.linear.app/graphql", bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("linear: %w", err)
	}
	req.Header.Set("authorization", l.APIKey)
	req.Header.Set("content-type", "application/json")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linear: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("linear: api error %d: %s", resp.StatusCode, respBody)
	}

	var result map[string]any
	json.Unmarshal(respBody, &result)

	if errs, ok := result["errors"]; ok {
		return nil, fmt.Errorf("linear: graphql errors: %v", errs)
	}

	return orchkit.Output{"result": result["data"], "status": resp.StatusCode}, nil
}
