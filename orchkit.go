package orchkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Input = map[string]any
type Output = map[string]any

type Schema struct {
	Description string
	Params      map[string]any
}

type Node interface {
	Name() string
	Schema() Schema
	Execute(ctx context.Context, in Input) (Output, error)
}

// ----------------------------------------------------------------------------
// Step — now supports timeout, condition, and parallel groups.
// ----------------------------------------------------------------------------

type Step struct {
	ID      string
	Node    Node
	In      map[string]string
	Out     map[string]string
	Timeout time.Duration      // 0 = no per-step timeout
	When    func(Input) bool   // nil = always run; return false to skip
}

// ----------------------------------------------------------------------------
// Flow — sequential steps with parallel groups and conditionals.
// ----------------------------------------------------------------------------

type Flow struct {
	steps    []step
}

// internal step wrapper — holds either a single Step or a parallel group
type step struct {
	single   *Step
	parallel []Step // non-nil = run concurrently, merge outputs
}

func NewFlow() *Flow { return &Flow{} }

// Step appends a sequential step.
func (f *Flow) Step(id string, node Node) *Flow {
	f.steps = append(f.steps, step{single: &Step{ID: id, Node: node}})
	return f
}

// StepWith appends a step with full config (timeout, condition, mapping).
func (f *Flow) StepWith(s Step) *Flow {
	f.steps = append(f.steps, step{single: &s})
	return f
}

// Parallel runs multiple steps concurrently and merges their outputs.
// All steps get the same input snapshot. Each writes under its own ID.
// If any step errors, the parallel group fails and the flow stops.
//
// Example:
//
//	flow.Parallel(
//	    orchkit.Step{ID: "a", Node: nodeA},
//	    orchkit.Step{ID: "b", Node: nodeB},
//	)
func (f *Flow) Parallel(steps ...Step) *Flow {
	f.steps = append(f.steps, step{parallel: steps})
	return f
}

// Steps returns all steps for inspection.
func (f *Flow) Steps() []step { return f.steps }

// ----------------------------------------------------------------------------
// Hooks — observe flow execution without modifying it.
// ----------------------------------------------------------------------------

type Hooks struct {
	OnStepStart  func(id string, in Input)
	OnStepEnd    func(id string, out Output, err error, elapsed time.Duration)
	OnFlowEnd    func(state map[string]any, err error)
}

// ----------------------------------------------------------------------------
// RunOptions — optional config for Run.
// ----------------------------------------------------------------------------

type RunOptions struct {
	Hooks *Hooks
}

// ----------------------------------------------------------------------------
// Run — executes a flow. Now supports parallel, conditionals, hooks, timeouts.
// ----------------------------------------------------------------------------

func Run(ctx context.Context, flow *Flow, store Store, opts ...RunOptions) (map[string]any, error) {
	if flow == nil {
		return nil, errors.New("orchkit: nil flow")
	}
	if store == nil {
		store = NewMemStore()
	}
	var hooks *Hooks
	if len(opts) > 0 {
		hooks = opts[0].Hooks
	}

	for _, s := range flow.steps {
		if s.parallel != nil {
			if err := runParallel(ctx, s.parallel, store, hooks); err != nil {
				return nil, err
			}
		} else {
			if err := runStep(ctx, s.single, store, hooks); err != nil {
				return nil, err
			}
		}
	}

	state, err := store.Snapshot(ctx)
	if hooks != nil && hooks.OnFlowEnd != nil {
		hooks.OnFlowEnd(state, err)
	}
	return state, err
}

func runStep(ctx context.Context, s *Step, store Store, hooks *Hooks) error {
	in, err := buildInput(ctx, *s, store)
	if err != nil {
		return fmt.Errorf("step %q: building input: %w", s.ID, err)
	}

	// Conditional — skip step if When returns false.
	if s.When != nil && !s.When(in) {
		return nil
	}

	// Per-step timeout.
	stepCtx := ctx
	var cancel context.CancelFunc
	if s.Timeout > 0 {
		stepCtx, cancel = context.WithTimeout(ctx, s.Timeout)
		defer cancel()
	}

	start := time.Now()
	if hooks != nil && hooks.OnStepStart != nil {
		hooks.OnStepStart(s.ID, in)
	}

	out, err := s.Node.Execute(stepCtx, in)
	elapsed := time.Since(start)

	if hooks != nil && hooks.OnStepEnd != nil {
		hooks.OnStepEnd(s.ID, out, err, elapsed)
	}

	if err != nil {
		return fmt.Errorf("step %q (%s): %w", s.ID, s.Node.Name(), err)
	}

	return writeOutput(ctx, *s, out, store)
}

func runParallel(ctx context.Context, steps []Step, store Store, hooks *Hooks) error {
	type result struct {
		id  string
		out Output
		err error
	}

	// Take a snapshot before parallel execution — all steps get the same input.
	snap, err := store.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("parallel: snapshot: %w", err)
	}

	results := make(chan result, len(steps))
	var wg sync.WaitGroup

	for _, s := range steps {
		wg.Add(1)
		go func(s Step) {
			defer wg.Done()

			// Build input from the pre-parallel snapshot.
			in := Input{}
			if s.In == nil {
				for k, v := range snap {
					in[k] = v
				}
			} else {
				for stateKey, nodeKey := range s.In {
					if v, ok := snap[stateKey]; ok {
						in[nodeKey] = v
					}
				}
			}

			// Conditional.
			if s.When != nil && !s.When(in) {
				results <- result{id: s.ID}
				return
			}

			stepCtx := ctx
			var cancel context.CancelFunc
			if s.Timeout > 0 {
				stepCtx, cancel = context.WithTimeout(ctx, s.Timeout)
				defer cancel()
			}

			start := time.Now()
			if hooks != nil && hooks.OnStepStart != nil {
				hooks.OnStepStart(s.ID, in)
			}

			out, err := s.Node.Execute(stepCtx, in)
			elapsed := time.Since(start)

			if hooks != nil && hooks.OnStepEnd != nil {
				hooks.OnStepEnd(s.ID, out, err, elapsed)
			}

			results <- result{id: s.ID, out: out, err: err}
			_ = elapsed
		}(s)
	}

	wg.Wait()
	close(results)

	// Collect results — fail fast on first error.
	var firstErr error
	for r := range results {
		if r.err != nil && firstErr == nil {
			firstErr = fmt.Errorf("parallel step %q: %w", r.id, r.err)
		}
		if r.err == nil && r.out != nil {
			// Find the step config for output mapping.
			for _, s := range steps {
				if s.ID == r.id {
					if werr := writeOutput(ctx, s, r.out, store); werr != nil && firstErr == nil {
						firstErr = werr
					}
					break
				}
			}
		}
	}
	return firstErr
}

func buildInput(ctx context.Context, s Step, store Store) (Input, error) {
	if s.In == nil {
		snap, err := store.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		return Input(snap), nil
	}
	in := Input{}
	for stateKey, nodeKey := range s.In {
		v, ok, err := store.Get(ctx, stateKey)
		if err != nil {
			return nil, err
		}
		if ok {
			in[nodeKey] = v
		}
	}
	return in, nil
}

func writeOutput(ctx context.Context, s Step, out Output, store Store) error {
	if out == nil {
		return nil
	}
	if s.Out == nil {
		return store.Put(ctx, s.ID, out)
	}
	for nodeKey, stateKey := range s.Out {
		if v, ok := out[nodeKey]; ok {
			if err := store.Put(ctx, stateKey, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// FlowNode — wraps a Flow as a Node. Compose flows inside flows.
// ----------------------------------------------------------------------------

type FlowNode struct {
	flow  *Flow
	store Store
}

// NewFlowNode wraps a Flow so it can be used as a step inside another Flow.
func NewFlowNode(flow *Flow, store Store) *FlowNode {
	return &FlowNode{flow: flow, store: store}
}

func (f *FlowNode) Name() string { return "flow" }
func (f *FlowNode) Schema() Schema {
	return Schema{Description: "Executes a sub-flow as a single step."}
}
func (f *FlowNode) Execute(ctx context.Context, in Input) (Output, error) {
	// Seed the sub-flow store with parent input.
	store := f.store
	if store == nil {
		store = NewMemStore()
	}
	for k, v := range in {
		if err := store.Put(ctx, k, v); err != nil {
			return nil, err
		}
	}
	return Run(ctx, f.flow, store)
}

// ----------------------------------------------------------------------------
// MemStore
// ----------------------------------------------------------------------------

type MemStore struct {
	mu   sync.RWMutex
	data map[string]any
}

func NewMemStore() *MemStore {
	return &MemStore{data: map[string]any{}}
}

func (m *MemStore) Get(_ context.Context, key string) (any, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *MemStore) Put(_ context.Context, key string, val any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = val
	return nil
}

func (m *MemStore) Snapshot(_ context.Context) (map[string]any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]any, len(m.data))
	for k, v := range m.data {
		out[k] = v
	}
	return out, nil
}

// ----------------------------------------------------------------------------
// Store — where flow state lives. One interface, swap implementations freely.
// ----------------------------------------------------------------------------

type Store interface {
	Get(ctx context.Context, key string) (any, bool, error)
	Put(ctx context.Context, key string, val any) error
	Snapshot(ctx context.Context) (map[string]any, error)
}
