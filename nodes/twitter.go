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

// Twitter posts tweets and reads timeline via Twitter API v2.
// Actions: tweet, get_timeline, get_tweet, delete_tweet.
// Requires Bearer token (read) or OAuth 2.0 user context (write).
//
// Example:
//
//	nodes.NewTwitter("bearer_token")
type Twitter struct {
	BearerToken string
	client      *http.Client
}

func NewTwitter(bearerToken string) *Twitter {
	return &Twitter{BearerToken: bearerToken, client: &http.Client{Timeout: 15 * time.Second}}
}

func (t *Twitter) Name() string { return "twitter" }

func (t *Twitter) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Posts and reads Twitter/X via API v2. Actions: tweet, get_timeline, get_tweet.",
		Params: map[string]any{
			"action": map[string]any{"type": "string", "desc": "tweet | get_timeline | get_tweet | delete_tweet"},
			"text":   map[string]any{"type": "string", "desc": "Tweet text (tweet action, max 280 chars)."},
			"id":     map[string]any{"type": "string", "desc": "Tweet ID (get_tweet, delete_tweet)."},
			"user":   map[string]any{"type": "string", "desc": "Username without @ (get_timeline)."},
		},
	}
}

func (t *Twitter) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	base := "https://api.twitter.com/2"

	switch action {
	case "tweet", "":
		text, _ := in["text"].(string)
		if text == "" {
			return nil, fmt.Errorf("twitter: text required for tweet")
		}
		if len([]rune(text)) > 280 {
			return nil, fmt.Errorf("twitter: text exceeds 280 characters")
		}
		return t.post(ctx, base+"/tweets", map[string]any{"text": text})

	case "get_tweet":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("twitter: id required")
		}
		return t.get(ctx, base+"/tweets/"+id)

	case "get_timeline":
		user, _ := in["user"].(string)
		if user == "" {
			return nil, fmt.Errorf("twitter: user required")
		}
		// First get user ID, then get timeline.
		userOut, err := t.get(ctx, base+"/users/by/username/"+user)
		if err != nil {
			return nil, err
		}
		result, _ := userOut["result"].(map[string]any)
		data, _ := result["data"].(map[string]any)
		userID, _ := data["id"].(string)
		return t.get(ctx, base+"/users/"+userID+"/tweets?max_results=10")

	case "delete_tweet":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("twitter: id required")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/tweets/"+id, nil)
		if err != nil {
			return nil, err
		}
		return t.do(req)

	default:
		return nil, fmt.Errorf("twitter: unknown action %q", action)
	}
}

func (t *Twitter) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return t.do(req)
}

func (t *Twitter) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return t.do(req)
}

func (t *Twitter) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+t.BearerToken)
	req.Header.Set("content-type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("twitter: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
