package nodes

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Env reads environment variables into the flow state.
// Safe: only reads keys you explicitly name. Never dumps all env.
//
// Example:
//
//	nodes.NewEnv("DATABASE_URL", "API_KEY", "PORT")
type Env struct {
	Keys []string // env var names to read
}

func NewEnv(keys ...string) *Env {
	return &Env{Keys: keys}
}

func (e *Env) Name() string { return "env" }

func (e *Env) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Reads named environment variables into the flow. Only reads explicitly named keys.",
		Params: map[string]any{
			"keys": map[string]any{"type": "array", "desc": "List of env var names to read."},
		},
	}
}

func (e *Env) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	keys := e.Keys
	if v, ok := in["keys"].([]any); ok && len(v) > 0 {
		keys = nil
		for _, k := range v {
			if s, ok := k.(string); ok {
				keys = append(keys, s)
			}
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("env: no keys specified")
	}

	out := orchkit.Output{}
	missing := []string{}
	for _, k := range keys {
		v := os.Getenv(k)
		if v == "" {
			missing = append(missing, k)
		}
		// Store under lowercased key for ergonomic access in templates/nodes.
		out[strings.ToLower(k)] = v
	}
	if len(missing) > 0 {
		out["missing"] = missing
	}
	return out, nil
}
