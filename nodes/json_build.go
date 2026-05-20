package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"orchkit"
)

// JSONBuild constructs a JSON object from selected input keys.
// Useful for shaping data before sending to an API or writing to a file.
//
// Example — pick specific keys from flow state:
//
//	nodes.NewJSONBuild("name", "email", "score")
//
// Example — rename keys using a map:
//
//	nodes.NewJSONBuildMapped(map[string]string{"user_name": "name", "user_email": "email"})
type JSONBuild struct {
	Keys   []string          // pick these keys from input as-is
	Mapped map[string]string // map input key -> output key (rename)
}

func NewJSONBuild(keys ...string) *JSONBuild {
	return &JSONBuild{Keys: keys}
}

func NewJSONBuildMapped(mapped map[string]string) *JSONBuild {
	return &JSONBuild{Mapped: mapped}
}

func (j *JSONBuild) Name() string { return "json_build" }

func (j *JSONBuild) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Constructs a JSON object by selecting and optionally renaming keys from the input.",
		Params: map[string]any{
			"keys":   map[string]any{"type": "array", "desc": "List of input keys to include in output."},
			"mapped": map[string]any{"type": "object", "desc": "Map of input_key -> output_key for renaming."},
		},
	}
}

func (j *JSONBuild) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	result := map[string]any{}

	// Keys from constructor.
	for _, k := range j.Keys {
		if v, ok := in[k]; ok {
			result[k] = v
		}
	}

	// Mapped keys from constructor.
	for inKey, outKey := range j.Mapped {
		if v, ok := in[inKey]; ok {
			result[outKey] = v
		}
	}

	// Keys from input at runtime.
	if keys, ok := in["keys"].([]any); ok {
		for _, k := range keys {
			if ks, ok := k.(string); ok {
				if v, exists := in[ks]; exists {
					result[ks] = v
				}
			}
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("json_build: no keys matched — specify keys in constructor or input")
	}

	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("json_build: marshal: %w", err)
	}

	return orchkit.Output{
		"json":   string(raw),
		"object": result,
	}, nil
}
