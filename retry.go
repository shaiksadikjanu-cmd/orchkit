package orchkit

import (
	"context"
	"fmt"
	"time"
)

// Retry wraps any Node and retries on error.
// Usage: retry.go is a policy wrapper — it takes a Node, returns a Node.
// It never touches the inner node's logic. Just wraps Execute.
//
// Example:
//
//	nodes.NewHTTPGet(url)                          // plain node
//	orchkit.Retry(nodes.NewHTTPGet(url), 3, time.Second) // with retry
type retryNode struct {
	inner    Node
	attempts int
	backoff  time.Duration
}

// Retry returns a Node that retries inner up to `attempts` times,
// waiting `backoff` between each attempt. If all attempts fail,
// the last error is returned with attempt count in the message.
func Retry(inner Node, attempts int, backoff time.Duration) Node {
	if attempts < 1 {
		attempts = 1
	}
	return &retryNode{
		inner:    inner,
		attempts: attempts,
		backoff:  backoff,
	}
}

// Name delegates to the inner node so the flow state key stays the same.
func (r *retryNode) Name() string { return r.inner.Name() }

// Schema delegates to the inner node — retry is invisible to the AI adapter.
func (r *retryNode) Schema() Schema { return r.inner.Schema() }

// Execute runs the inner node, retrying on error with backoff.
func (r *retryNode) Execute(ctx context.Context, in Input) (Output, error) {
	var lastErr error
	for attempt := 1; attempt <= r.attempts; attempt++ {
		out, err := r.inner.Execute(ctx, in)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if attempt < r.attempts {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("retry: context cancelled after %d/%d attempts: %w", attempt, r.attempts, ctx.Err())
			case <-time.After(r.backoff):
			}
		}
	}
	return nil, fmt.Errorf("retry: failed after %d attempts: %w", r.attempts, lastErr)
}
