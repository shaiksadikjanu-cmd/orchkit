package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Webhook starts a temporary HTTP server that waits for one inbound POST
// request, captures the body, then shuts down.
//
// This is an inbound trigger — it lets an external system start a flow step.
// The server listens until a request arrives or the context is cancelled.
//
// Example:
//
//	nodes.NewWebhook(":8080", "/hook", 60*time.Second)
type Webhook struct {
	Addr    string        // e.g. ":8080"
	Path    string        // e.g. "/hook"
	Timeout time.Duration // how long to wait for a request
}

func NewWebhook(addr, path string, timeout time.Duration) *Webhook {
	return &Webhook{Addr: addr, Path: path, Timeout: timeout}
}

func (w *Webhook) Name() string { return "webhook" }

func (w *Webhook) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Starts a temporary HTTP server and waits for one inbound POST request. Returns the request body.",
		Params: map[string]any{
			"addr":    map[string]any{"type": "string", "desc": "Address to listen on, e.g. :8080"},
			"path":    map[string]any{"type": "string", "desc": "URL path to receive on, e.g. /hook"},
			"timeout": map[string]any{"type": "number", "desc": "Seconds to wait for a request."},
		},
	}
}

func (w *Webhook) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	addr := w.Addr
	if v, ok := in["addr"].(string); ok && v != "" {
		addr = v
	}
	if addr == "" {
		addr = ":8080"
	}

	path := w.Path
	if v, ok := in["path"].(string); ok && v != "" {
		path = v
	}
	if path == "" {
		path = "/hook"
	}

	timeout := w.Timeout
	if v, ok := in["timeout"].(float64); ok && v > 0 {
		timeout = time.Duration(v) * time.Second
	}
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	type result struct {
		body    string
		headers map[string]string
		err     error
	}

	ch := make(chan result, 1)
	var once sync.Once

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		headers := map[string]string{}
		for k := range r.Header {
			headers[k] = r.Header.Get(k)
		}
		once.Do(func() {
			ch <- result{body: string(body), headers: headers, err: err}
		})
		rw.WriteHeader(http.StatusOK)
		fmt.Fprint(rw, `{"received":true}`)
	})

	srv := &http.Server{Addr: addr, Handler: mux}

	// Start server in background.
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Apply timeout on top of context.
	waitCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var res result
	select {
	case err := <-errCh:
		return nil, fmt.Errorf("webhook: server error: %w", err)
	case res = <-ch:
	case <-waitCtx.Done():
		srv.Shutdown(context.Background())
		return nil, fmt.Errorf("webhook: timed out waiting for request: %w", waitCtx.Err())
	}

	srv.Shutdown(context.Background())

	if res.err != nil {
		return nil, fmt.Errorf("webhook: reading body: %w", res.err)
	}

	// Try to parse body as JSON — fall back to raw string.
	out := orchkit.Output{
		"raw":     res.body,
		"headers": res.headers,
	}
	var parsed any
	if json.Unmarshal([]byte(res.body), &parsed) == nil {
		out["body"] = parsed
	} else {
		out["body"] = res.body
	}

	return out, nil
}
