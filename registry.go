package orchkit

import (
	"fmt"
	"sync"
)

// Registry maps node names to Node constructors.
// It is the bridge between YAML flow definitions and Go node implementations.
//
// Usage:
//
//	reg := orchkit.NewRegistry()
//	reg.Register("http_get", func() Node { return nodes.NewHTTPGet("") })
//	reg.Register("json_parse", func() Node { return nodes.NewJSONParse("") })
//
//	flow, _ := orchkit.LoadYAML("flow.yaml")
//	orchkit.RunYAML(ctx, flow, reg.Build(), store)

type Registry struct {
	mu           sync.RWMutex
	constructors map[string]func() Node
}

func NewRegistry() *Registry {
	return &Registry{constructors: map[string]func() Node{}}
}

// Register adds a node constructor to the registry.
func (r *Registry) Register(name string, constructor func() Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.constructors[name] = constructor
}

// Build returns a snapshot map of name -> Node instance.
func (r *Registry) Build() map[string]Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]Node, len(r.constructors))
	for name, fn := range r.constructors {
		out[name] = fn()
	}
	return out
}

// Names returns all registered node names — useful for docs and tooling.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.constructors))
	for name := range r.constructors {
		names = append(names, name)
	}
	return names
}

// MustGet returns a node by name or panics — for use in tests.
func (r *Registry) MustGet(name string) Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.constructors[name]
	if !ok {
		panic(fmt.Sprintf("orchkit registry: node %q not registered", name))
	}
	return fn()
}
