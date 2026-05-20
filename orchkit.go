// Package orchkit is a composable orchestration kernel.
//
// The entire kernel lives in this one file. Read it top to bottom and you
// understand the whole system. Every concept here is small on purpose.
package orchkit

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ----------------------------------------------------------------------------
// Node — the unit of work. Every "arm" is a Node.
// ----------------------------------------------------------------------------

// Input and Output are intentionally untyped (map[string]any). This keeps
// every node uniform, makes the AI adapter trivial (map == JSON), and lets
// you change a node's shape without cascading edits.
type Input = map[string]any
type Output = map[string]any

// Schema describes a node for humans and for AI tool-use.
type Schema struct {
	Description string         // what the node does, in plain English
	Params      map[string]any // JSON-schema-ish: field name -> {"type": "string", "desc": "..."}
}

// Node is the only contract you must implement to be usable in a flow.
type Node interface {
	Name() string
	Schema() Schema
	Execute(ctx context.Context, in Input) (Output, error)
}

// ----------------------------------------------------------------------------
// Flow — an ordered list of steps. No DAGs, no FSMs, just sequence + branches.
// Branching/parallel can be added later when actually needed.
// ----------------------------------------------------------------------------

// Step binds a Node to an ID inside a Flow and optionally maps its output
// into the flow's shared state.
type Step struct {
	ID   string
	Node Node
	// In maps flow-state keys -> node-input keys. If nil, the entire state
	// is passed as input. Keeps wiring explicit but ergonomic.
	In map[string]string
	// Out maps node-output keys -> flow-state keys. If nil, output is merged
	// into state under the step ID as a namespace.
	Out map[string]string
}

// Flow is built with a fluent builder. No hidden state, no init().
type Flow struct {
	steps []Step
}

// NewFlow starts a new flow.
func NewFlow() *Flow { return &Flow{} }

// Step appends a step. The simplest form: f.Step("fetch", http.Get(url)).
func (f *Flow) Step(id string, node Node) *Flow {
	f.steps = append(f.steps, Step{ID: id, Node: node})
	return f
}

// StepWith appends a step with explicit input/output mapping.
func (f *Flow) StepWith(s Step) *Flow {
	f.steps = append(f.steps, s)
	return f
}

// Steps returns the steps, mostly for inspection/testing.
func (f *Flow) Steps() []Step { return f.steps }

// ----------------------------------------------------------------------------
// Store — where flow state lives. One interface, swap implementations freely.
// ----------------------------------------------------------------------------

// Store is the persistence boundary. MemStore is default; add boltstore.go,
// pgstore.go etc. later without changing anything else.
type Store interface {
	Get(ctx context.Context, key string) (any, bool, error)
	Put(ctx context.Context, key string, val any) error
	Snapshot(ctx context.Context) (map[string]any, error)
}

// ----------------------------------------------------------------------------
// Run — executes a flow against a store. Plain sequential. That's the point.
// ----------------------------------------------------------------------------

// Run executes every step in order. If a step errors, the run stops and
// the error is returned with context about which step failed.
func Run(ctx context.Context, flow *Flow, store Store) (map[string]any, error) {
	if flow == nil {
		return nil, errors.New("orchkit: nil flow")
	}
	if store == nil {
		store = NewMemStore()
	}

	for _, step := range flow.steps {
		in, err := buildInput(ctx, step, store)
		if err != nil {
			return nil, fmt.Errorf("step %q: building input: %w", step.ID, err)
		}

		out, err := step.Node.Execute(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("step %q (%s): %w", step.ID, step.Node.Name(), err)
		}

		if err := writeOutput(ctx, step, out, store); err != nil {
			return nil, fmt.Errorf("step %q: writing output: %w", step.ID, err)
		}
	}

	return store.Snapshot(ctx)
}

func buildInput(ctx context.Context, step Step, store Store) (Input, error) {
	if step.In == nil {
		snap, err := store.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		return Input(snap), nil
	}
	in := Input{}
	for stateKey, nodeKey := range step.In {
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

func writeOutput(ctx context.Context, step Step, out Output, store Store) error {
	if out == nil {
		return nil
	}
	if step.Out == nil {
		// Namespace under the step ID so two steps can't clobber each other.
		return store.Put(ctx, step.ID, out)
	}
	for nodeKey, stateKey := range step.Out {
		if v, ok := out[nodeKey]; ok {
			if err := store.Put(ctx, stateKey, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// ----------------------------------------------------------------------------
// MemStore — the default in-memory Store. ~30 lines.
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
