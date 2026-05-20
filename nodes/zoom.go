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

// Zoom interacts with the Zoom API v2.
// Actions: create_meeting, list_meetings, get_meeting, delete_meeting.
// Requires a Server-to-Server OAuth token.
//
// Example:
//
//	nodes.NewZoom("access_token")
type Zoom struct {
	Token  string
	client *http.Client
}

func NewZoom(token string) *Zoom {
	return &Zoom{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (z *Zoom) Name() string { return "zoom" }

func (z *Zoom) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages Zoom meetings via API v2. Actions: create_meeting, list_meetings, get_meeting, delete_meeting.",
		Params: map[string]any{
			"action":    map[string]any{"type": "string", "desc": "create_meeting | list_meetings | get_meeting | delete_meeting"},
			"topic":     map[string]any{"type": "string", "desc": "Meeting topic (create_meeting)."},
			"start":     map[string]any{"type": "string", "desc": "Start time ISO8601 e.g. 2026-05-21T10:00:00Z (create_meeting)."},
			"duration":  map[string]any{"type": "integer", "desc": "Duration in minutes. Default 60."},
			"meeting_id": map[string]any{"type": "string", "desc": "Meeting ID (get_meeting, delete_meeting)."},
		},
	}
}

func (z *Zoom) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := "https://api.zoom.us/v2"
	action, _ := in["action"].(string)

	switch action {
	case "create_meeting", "":
		topic, _ := in["topic"].(string)
		if topic == "" {
			return nil, fmt.Errorf("zoom: topic required")
		}
		duration := 60
		if v, ok := in["duration"].(float64); ok {
			duration = int(v)
		}
		body := map[string]any{
			"topic":    topic,
			"type":     2, // Scheduled meeting
			"duration": duration,
			"settings": map[string]any{
				"join_before_host": true,
				"waiting_room":     false,
			},
		}
		if start, ok := in["start"].(string); ok {
			body["start_time"] = start
		}
		return z.post(ctx, base+"/users/me/meetings", body)

	case "list_meetings":
		return z.get(ctx, base+"/users/me/meetings?page_size=10")

	case "get_meeting":
		id, _ := in["meeting_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("zoom: meeting_id required")
		}
		return z.get(ctx, base+"/meetings/"+id)

	case "delete_meeting":
		id, _ := in["meeting_id"].(string)
		if id == "" {
			return nil, fmt.Errorf("zoom: meeting_id required")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/meetings/"+id, nil)
		if err != nil {
			return nil, err
		}
		return z.do(req)

	default:
		return nil, fmt.Errorf("zoom: unknown action %q", action)
	}
}

func (z *Zoom) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return z.do(req)
}

func (z *Zoom) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return z.do(req)
}

func (z *Zoom) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+z.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := z.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zoom: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("zoom: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	if len(body) > 0 {
		json.Unmarshal(body, &result)
	}
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
