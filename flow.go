package orchkit

import (
	"context"
	"fmt"
	"time"
)

type YAMLFlow struct {
	Name  string     `yaml:"name"`
	Steps []YAMLStep `yaml:"steps"`
}

type YAMLStep struct {
	ID       string         `yaml:"id"`
	Node     string         `yaml:"node"`
	Input    map[string]any `yaml:"input"`
	Timeout  string         `yaml:"timeout"`
	When     string         `yaml:"when"`
	Parallel []YAMLStep     `yaml:"parallel"`
}

// RunYAML executes a YAMLFlow. Supports ${step.field} interpolation.
func RunYAML(ctx context.Context, flow YAMLFlow, registry map[string]Node, store Store, opts ...RunOptions) (map[string]any, error) {
	if store == nil {
		store = NewMemStore()
	}

	var hooks *Hooks
	if len(opts) > 0 {
		hooks = opts[0].Hooks
	}

	for _, s := range flow.Steps {
		node, ok := registry[s.Node]
		if !ok {
			return nil, fmt.Errorf("orchkit: unknown node %q in step %q", s.Node, s.ID)
		}

		// Snapshot CURRENT state — includes all previous step outputs.
		state, err := store.Snapshot(ctx)
		if err != nil {
			return nil, fmt.Errorf("step %q: snapshot: %w", s.ID, err)
		}

		// Resolve ${step.field} references using current state.
		// This happens AFTER previous steps have written their outputs.
		resolvedInput := InterpolateStep(s.Input, state)

		// Build the node's input — merge state + resolved step inputs.
		// Step-level inputs take priority over inherited state.
		in := make(Input)
		for k, v := range state {
			in[k] = v
		}
		for k, v := range resolvedInput {
			in[k] = v
		}

		// Parse timeout.
		var timeout time.Duration
		if s.Timeout != "" {
			timeout, err = time.ParseDuration(s.Timeout)
			if err != nil {
				return nil, fmt.Errorf("step %q: invalid timeout %q: %w", s.ID, s.Timeout, err)
			}
		}

		stepCtx := ctx
		var cancel context.CancelFunc
		if timeout > 0 {
			stepCtx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}

		start := time.Now()
		if hooks != nil && hooks.OnStepStart != nil {
			hooks.OnStepStart(s.ID, in)
		}

		out, err := node.Execute(stepCtx, in)
		elapsed := time.Since(start)

		if hooks != nil && hooks.OnStepEnd != nil {
			hooks.OnStepEnd(s.ID, out, err, elapsed)
		}

		if err != nil {
			return nil, fmt.Errorf("step %q (%s): %w", s.ID, node.Name(), err)
		}

		// Write output under step ID namespace.
		if out != nil {
			if err := store.Put(ctx, s.ID, out); err != nil {
				return nil, fmt.Errorf("step %q: writing output: %w", s.ID, err)
			}
		}
	}

	state, err := store.Snapshot(ctx)
	if hooks != nil && hooks.OnFlowEnd != nil {
		hooks.OnFlowEnd(state, err)
	}
	return state, err
}
