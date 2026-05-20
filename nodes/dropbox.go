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

// Dropbox manages files via Dropbox API v2.
// Actions: upload, download, list, delete, move, copy, get_link.
//
// Example:
//
//	nodes.NewDropbox("sl.your_access_token")
type Dropbox struct {
	Token  string
	client *http.Client
}

func NewDropbox(token string) *Dropbox {
	return &Dropbox{Token: token, client: &http.Client{Timeout: 60 * time.Second}}
}

func (d *Dropbox) Name() string { return "dropbox" }

func (d *Dropbox) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Manages files in Dropbox. Actions: upload, download, list, delete, move, copy, get_link.",
		Params: map[string]any{
			"action":  map[string]any{"type": "string", "desc": "upload | download | list | delete | move | copy | get_link"},
			"path":    map[string]any{"type": "string", "desc": "File or folder path e.g. /folder/file.txt"},
			"content": map[string]any{"type": "string", "desc": "File content to upload (upload action)."},
			"to_path": map[string]any{"type": "string", "desc": "Destination path (move, copy)."},
		},
	}
}

func (d *Dropbox) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	action, _ := in["action"].(string)
	path, _ := in["path"].(string)

	switch action {
	case "list", "":
		if path == "" {
			path = ""
		}
		return d.api(ctx, "https://api.dropboxapi.com/2/files/list_folder",
			map[string]any{"path": path, "recursive": false})

	case "upload":
		content, _ := in["content"].(string)
		if path == "" || content == "" {
			return nil, fmt.Errorf("dropbox: path and content required for upload")
		}
		arg, _ := json.Marshal(map[string]any{"path": path, "mode": "overwrite"})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://content.dropboxapi.com/2/files/upload",
			bytes.NewReader([]byte(content)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("authorization", "Bearer "+d.Token)
		req.Header.Set("content-type", "application/octet-stream")
		req.Header.Set("dropbox-api-arg", string(arg))
		resp, err := d.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("dropbox: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("dropbox: upload error %d: %s", resp.StatusCode, body)
		}
		var result any
		json.Unmarshal(body, &result)
		return orchkit.Output{"result": result, "uploaded": true}, nil

	case "download":
		if path == "" {
			return nil, fmt.Errorf("dropbox: path required for download")
		}
		arg, _ := json.Marshal(map[string]any{"path": path})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://content.dropboxapi.com/2/files/download", nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("authorization", "Bearer "+d.Token)
		req.Header.Set("dropbox-api-arg", string(arg))
		resp, err := d.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("dropbox: %w", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("dropbox: download error %d: %s", resp.StatusCode, body)
		}
		return orchkit.Output{"content": string(body), "path": path}, nil

	case "delete":
		if path == "" {
			return nil, fmt.Errorf("dropbox: path required for delete")
		}
		return d.api(ctx, "https://api.dropboxapi.com/2/files/delete_v2",
			map[string]any{"path": path})

	case "move":
		toPath, _ := in["to_path"].(string)
		if path == "" || toPath == "" {
			return nil, fmt.Errorf("dropbox: path and to_path required for move")
		}
		return d.api(ctx, "https://api.dropboxapi.com/2/files/move_v2",
			map[string]any{"from_path": path, "to_path": toPath})

	case "copy":
		toPath, _ := in["to_path"].(string)
		if path == "" || toPath == "" {
			return nil, fmt.Errorf("dropbox: path and to_path required for copy")
		}
		return d.api(ctx, "https://api.dropboxapi.com/2/files/copy_v2",
			map[string]any{"from_path": path, "to_path": toPath})

	case "get_link":
		if path == "" {
			return nil, fmt.Errorf("dropbox: path required for get_link")
		}
		return d.api(ctx, "https://api.dropboxapi.com/2/sharing/create_shared_link_with_settings",
			map[string]any{"path": path})

	default:
		return nil, fmt.Errorf("dropbox: unknown action %q", action)
	}
}

func (d *Dropbox) api(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("authorization", "Bearer "+d.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dropbox: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("dropbox: api error %d: %s", resp.StatusCode, respBody)
	}
	var result any
	json.Unmarshal(respBody, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}
