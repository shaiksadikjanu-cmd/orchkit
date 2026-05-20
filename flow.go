package orchkit

import (
	"context"
	"fmt"
	"time"
)

// YAMLFlow defines a flow in a declarative format.
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

// RunYAML executes a YAMLFlow using a node registry.
// Supports ${step.field} interpolation between steps.
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

		// Get current state for interpolation.
		state, err := store.Snapshot(ctx)
		if err != nil {
			return nil, fmt.Errorf("step %q: snapshot: %w", s.ID, err)
		}

		// Resolve ${step.field} and ${ENV_VAR} in input values.
		resolvedInput := InterpolateStep(s.Input, state)

		// Seed store with resolved static inputs.
		for k, v := range resolvedInput {
			store.Put(ctx, k, v)
		}

		step := Step{ID: s.ID, Node: node}

		if s.Timeout != "" {
			d, err := time.ParseDuration(s.Timeout)
			if err != nil {
				return nil, fmt.Errorf("step %q: invalid timeout %q: %w", s.ID, s.Timeout, err)
			}
			step.Timeout = d
		}

		in, err := buildInput(ctx, step, store)
		if err != nil {
			return nil, fmt.Errorf("step %q: building input: %w", s.ID, err)
		}

		start := time.Now()
		if hooks != nil && hooks.OnStepStart != nil {
			hooks.OnStepStart(s.ID, in)
		}

		out, err := node.Execute(ctx, in)
		elapsed := time.Since(start)

		if hooks != nil && hooks.OnStepEnd != nil {
			hooks.OnStepEnd(s.ID, out, err, elapsed)
		}

		if err != nil {
			return nil, fmt.Errorf("step %q (%s): %w", s.ID, node.Name(), err)
		}

		if err := writeOutput(ctx, step, out, store); err != nil {
			return nil, fmt.Errorf("step %q: writing output: %w", s.ID, err)
		}
	}

	state, err := store.Snapshot(ctx)
	if hooks != nil && hooks.OnFlowEnd != nil {
		hooks.OnFlowEnd(state, err)
	}
	return state, err
}
