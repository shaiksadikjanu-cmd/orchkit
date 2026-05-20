package nodes

import (
	"context"
	"fmt"
	"time"

	"orchkit"
)

// Delay pauses execution for a fixed duration.
// Useful for rate limiting between API calls, backoff, or scheduling.
//
// Example:
//
//	nodes.NewDelay(2 * time.Second)
type Delay struct {
	Duration time.Duration // if zero, taken from input["seconds"] at runtime
}

func NewDelay(d time.Duration) *Delay {
	return &Delay{Duration: d}
}

func (d *Delay) Name() string { return "delay" }

func (d *Delay) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Pauses execution for a given number of seconds. Useful for rate limiting.",
		Params: map[string]any{
			"seconds": map[string]any{"type": "number", "desc": "Seconds to wait. Falls back to constructor value."},
		},
	}
}

func (d *Delay) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	dur := d.Duration
	if v, ok := in["seconds"].(float64); ok && v > 0 {
		dur = time.Duration(v * float64(time.Second))
	}
	if dur <= 0 {
		return nil, fmt.Errorf("delay: duration must be > 0")
	}

	select {
	case <-time.After(dur):
		return orchkit.Output{"waited_ms": dur.Milliseconds()}, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("delay: context cancelled: %w", ctx.Err())
	}
}
