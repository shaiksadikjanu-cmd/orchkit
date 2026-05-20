package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"orchkit"
)

// Spotify interacts with the Spotify Web API.
// Actions: search, get_track, get_playlist, play, pause,
//          add_to_queue, get_current_track.
// Requires OAuth2 access token.
//
// Example:
//
//	nodes.NewSpotify("your_access_token")
type Spotify struct {
	Token  string
	client *http.Client
}

func NewSpotify(token string) *Spotify {
	return &Spotify{Token: token, client: &http.Client{Timeout: 15 * time.Second}}
}

func (s *Spotify) Name() string { return "spotify" }

func (s *Spotify) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Interacts with Spotify. Actions: search, get_track, get_playlist, play, pause, add_to_queue, get_current_track.",
		Params: map[string]any{
			"action":      map[string]any{"type": "string", "desc": "search | get_track | get_playlist | play | pause | add_to_queue | get_current_track"},
			"query":       map[string]any{"type": "string", "desc": "Search query (search action)."},
			"type":        map[string]any{"type": "string", "desc": "Search type: track | artist | album | playlist. Default track."},
			"id":          map[string]any{"type": "string", "desc": "Track or playlist ID."},
			"uri":         map[string]any{"type": "string", "desc": "Spotify URI e.g. spotify:track:xxx (play, add_to_queue)."},
			"device_id":   map[string]any{"type": "string", "desc": "Device ID for playback (optional)."},
			"limit":       map[string]any{"type": "integer", "desc": "Max search results. Default 10."},
		},
	}
}

func (s *Spotify) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	base := "https://api.spotify.com/v1"
	action, _ := in["action"].(string)

	switch action {
	case "search", "":
		query, _ := in["query"].(string)
		if query == "" {
			return nil, fmt.Errorf("spotify: query required for search")
		}
		searchType, _ := in["type"].(string)
		if searchType == "" {
			searchType = "track"
		}
		limit := 10
		if v, ok := in["limit"].(float64); ok {
			limit = int(v)
		}
		return s.get(ctx, fmt.Sprintf("%s/search?q=%s&type=%s&limit=%d",
			base, url.QueryEscape(query), searchType, limit))

	case "get_track":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("spotify: id required")
		}
		return s.get(ctx, base+"/tracks/"+id)

	case "get_playlist":
		id, _ := in["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("spotify: id required")
		}
		return s.get(ctx, base+"/playlists/"+id)

	case "get_current_track":
		return s.get(ctx, base+"/me/player/currently-playing")

	case "play":
		body := map[string]any{}
		if uri, ok := in["uri"].(string); ok && uri != "" {
			body["uris"] = []string{uri}
		}
		playURL := base + "/me/player/play"
		if deviceID, ok := in["device_id"].(string); ok && deviceID != "" {
			playURL += "?device_id=" + deviceID
		}
		return s.put(ctx, playURL, body)

	case "pause":
		pauseURL := base + "/me/player/pause"
		if deviceID, ok := in["device_id"].(string); ok && deviceID != "" {
			pauseURL += "?device_id=" + deviceID
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, pauseURL, nil)
		if err != nil {
			return nil, err
		}
		return s.do(req)

	case "add_to_queue":
		uri, _ := in["uri"].(string)
		if uri == "" {
			return nil, fmt.Errorf("spotify: uri required for add_to_queue")
		}
		queueURL := base + "/me/player/queue?uri=" + url.QueryEscape(uri)
		if deviceID, ok := in["device_id"].(string); ok && deviceID != "" {
			queueURL += "&device_id=" + deviceID
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, queueURL,
			strings.NewReader(""))
		if err != nil {
			return nil, err
		}
		return s.do(req)

	default:
		return nil, fmt.Errorf("spotify: unknown action %q", action)
	}
}

func (s *Spotify) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return s.do(req)
}

func (s *Spotify) put(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return s.do(req)
}

func (s *Spotify) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+s.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("spotify: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("spotify: api error %d: %s", resp.StatusCode, body)
	}
	if resp.StatusCode == 204 {
		return orchkit.Output{"success": true}, nil
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
