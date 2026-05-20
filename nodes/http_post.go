package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// HTTPPost sends POST, PUT, or PATCH requests with a body.
// Method defaults to POST. Body can be JSON or plain string.
type HTTPPost struct {
	URL    string
	Method string // POST, PUT, PATCH — defaults to POST
	Client *http.Client
	Header http.Header
}

func NewHTTPPost(url string) *HTTPPost {
	return &HTTPPost{URL: url, Method: "POST"}
}

func NewHTTPPut(url string) *HTTPPost {
	return &HTTPPost{URL: url, Method: "PUT"}
}

func NewHTTPPatch(url string) *HTTPPost {
	return &HTTPPost{URL: url, Method: "PATCH"}
}

func (h *HTTPPost) Name() string { return "http_post" }

func (h *HTTPPost) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends an HTTP POST/PUT/PATCH request with a JSON or string body.",
		Params: map[string]any{
			"url":    map[string]any{"type": "string", "desc": "URL to send request to."},
			"body":   map[string]any{"type": "any", "desc": "Request body. Object = JSON, string = raw."},
			"method": map[string]any{"type": "string", "desc": "HTTP method: POST, PUT, PATCH. Defaults to POST."},
		},
	}
}

func (h *HTTPPost) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	url := h.URL
	if v, ok := in["url"].(string); ok && v != "" {
		url = v
	}
	if url == "" {
		return nil, fmt.Errorf("http_post: no URL provided")
	}

	method := h.Method
	if v, ok := in["method"].(string); ok && v != "" {
		method = strings.ToUpper(v)
	}
	if method == "" {
		method = "POST"
	}

	// Build body — accept map (serialize to JSON) or raw string.
	var bodyReader io.Reader
	contentType := "application/json"
	if body, ok := in["body"]; ok {
		switch v := body.(type) {
		case string:
			bodyReader = strings.NewReader(v)
			contentType = "text/plain"
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("http_post: marshalling body: %w", err)
			}
			bodyReader = bytes.NewReader(b)
		}
	}

	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("http_post: %w", err)
	}
	req.Header.Set("content-type", contentType)
	for k, vs := range h.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http_post: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("http_post: reading response: %w", err)
	}

	return orchkit.Output{
		"status": resp.StatusCode,
		"body":   string(respBody),
	}, nil
}
