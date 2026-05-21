package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Reddit interacts with the Reddit API.
// Actions: hot_posts, new_posts, search, get_post, submit_post.
// Read operations work with just a client_id + client_secret (app-only OAuth).
//
// Example:
//
//	nodes.NewReddit("client_id", "client_secret", "username", "password")
type Reddit struct {
	ClientID     string
	ClientSecret string
	Username     string
	Password     string
	accessToken  string
	client       *http.Client
}

func NewReddit(clientID, clientSecret, username, password string) *Reddit {
	return &Reddit{
		ClientID: clientID, ClientSecret: clientSecret,
		Username: username, Password: password,
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (r *Reddit) Name() string { return "reddit" }

func (r *Reddit) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Reads and posts to Reddit. Actions: hot_posts, new_posts, search, get_post, submit_post.",
		Params: map[string]any{
			"action":    map[string]any{"type": "string", "desc": "hot_posts | new_posts | search | get_post | submit_post"},
			"subreddit": map[string]any{"type": "string", "desc": "Subreddit name without r/ e.g. 'golang'."},
			"query":     map[string]any{"type": "string", "desc": "Search query (search action)."},
			"post_id":   map[string]any{"type": "string", "desc": "Post ID (get_post)."},
			"title":     map[string]any{"type": "string", "desc": "Post title (submit_post)."},
			"text":      map[string]any{"type": "string", "desc": "Post text body (submit_post)."},
			"limit":     map[string]any{"type": "number", "desc": "Max results. Default 10."},
		},
	}
}

func (r *Reddit) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	if err := r.ensureToken(ctx); err != nil {
		return nil, err
	}

	action, _ := in["action"].(string)
	subreddit, _ := in["subreddit"].(string)
	limit := 10
	if v, ok := in["limit"].(float64); ok {
		limit = int(v)
	}

	switch action {
	case "hot_posts", "":
		if subreddit == "" {
			return nil, fmt.Errorf("reddit: subreddit required")
		}
		return r.get(ctx, fmt.Sprintf("https://oauth.reddit.com/r/%s/hot?limit=%d", subreddit, limit))

	case "new_posts":
		if subreddit == "" {
			return nil, fmt.Errorf("reddit: subreddit required")
		}
		return r.get(ctx, fmt.Sprintf("https://oauth.reddit.com/r/%s/new?limit=%d", subreddit, limit))

	case "search":
		query, _ := in["query"].(string)
		if query == "" {
			return nil, fmt.Errorf("reddit: query required for search")
		}
		searchURL := fmt.Sprintf("https://oauth.reddit.com/search?q=%s&limit=%d", url.QueryEscape(query), limit)
		if subreddit != "" {
			searchURL += "&restrict_sr=true&sr=" + subreddit
		}
		return r.get(ctx, searchURL)

	case "get_post":
		postID, _ := in["post_id"].(string)
		if postID == "" {
			return nil, fmt.Errorf("reddit: post_id required")
		}
		return r.get(ctx, "https://oauth.reddit.com/comments/"+postID)

	case "submit_post":
		if subreddit == "" {
			return nil, fmt.Errorf("reddit: subreddit required for submit_post")
		}
		title, _ := in["title"].(string)
		text, _ := in["text"].(string)
		if title == "" {
			return nil, fmt.Errorf("reddit: title required for submit_post")
		}
		params := url.Values{}
		params.Set("sr", subreddit)
		params.Set("kind", "self")
		params.Set("title", title)
		params.Set("text", text)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://oauth.reddit.com/api/submit",
			strings.NewReader(params.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("content-type", "application/x-www-form-urlencoded")
		return r.do(req)

	default:
		return nil, fmt.Errorf("reddit: unknown action %q", action)
	}
}

func (r *Reddit) ensureToken(ctx context.Context) error {
	if r.accessToken != "" {
		return nil
	}
	params := url.Values{}
	params.Set("grant_type", "password")
	params.Set("username", r.Username)
	params.Set("password", r.Password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://www.reddit.com/api/v1/access_token",
		strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(r.ClientID, r.ClientSecret)
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	req.Header.Set("user-agent", "github.com/shaiksadikjanu-cmd/orchkit/1.0")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("reddit: auth: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		AccessToken string `json:"access_token"`
	}
	json.Unmarshal(body, &result)
	r.accessToken = result.AccessToken
	return nil
}

func (r *Reddit) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return r.do(req)
}

func (r *Reddit) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+r.accessToken)
	req.Header.Set("user-agent", "github.com/shaiksadikjanu-cmd/orchkit/1.0")
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("reddit: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("reddit: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
