// Package nodes provides standard "arms" — each file is one self-contained node.
// Rule: a node imports only the kernel and the standard library. Never another node.
package nodes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"orchkit"
)

// HTTPGet performs a GET request and returns status + body.
type HTTPGet struct {
	URL    string       // if empty, taken from input["url"]
	Client *http.Client // optional; defaults to a 30s client
	Header http.Header  // optional headers
}

// NewHTTPGet is the constructor. Keep node construction obvious.
func NewHTTPGet(url string) *HTTPGet {
	return &HTTPGet{URL: url}
}

func (h *HTTPGet) Name() string { return "http_get" }

func (h *HTTPGet) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Performs an HTTP GET request and returns the response body and status.",
		Params: map[string]any{
			"url": map[string]any{"type": "string", "desc": "URL to GET. Falls back to constructor URL if absent."},
		},
	}
}

func (h *HTTPGet) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	url := h.URL
	if v, ok := in["url"].(string); ok && v != "" {
		url = v
	}
	if url == "" {
		return nil, fmt.Errorf("http_get: no URL provided")
	}

	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range h.Header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return orchkit.Output{
		"status": resp.StatusCode,
		"body":   string(body),
	}, nil
}
