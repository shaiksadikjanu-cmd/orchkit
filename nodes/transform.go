package nodes

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// JSONParse parses a JSON string into a map and optionally extracts a single field.
type JSONParse struct {
	Field string // optional: if set, returns only this top-level field
}

func NewJSONParse(field string) *JSONParse {
	return &JSONParse{Field: field}
}

func (j *JSONParse) Name() string { return "json_parse" }

func (j *JSONParse) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Parses a JSON string. If field is set, returns only that field.",
		Params: map[string]any{
			"body":  map[string]any{"type": "string", "desc": "JSON string to parse."},
			"field": map[string]any{"type": "string", "desc": "Optional top-level field to extract."},
		},
	}
}

func (j *JSONParse) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	raw, _ := in["body"].(string)
	if raw == "" {
		// Try to find a body under a step namespace (e.g. previous "fetch" step).
		for _, v := range in {
			if m, ok := v.(map[string]any); ok {
				if b, ok := m["body"].(string); ok && b != "" {
					raw = b
					break
				}
			}
		}
	}
	if raw == "" {
		return nil, fmt.Errorf("json_parse: no body to parse")
	}

	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("json_parse: %w", err)
	}

	field := j.Field
	if v, ok := in["field"].(string); ok && v != "" {
		field = v
	}

	if field != "" {
		if m, ok := parsed.(map[string]any); ok {
			if v, ok := m[field]; ok {
				return orchkit.Output{"value": v}, nil
			}
			return nil, fmt.Errorf("json_parse: field %q not found", field)
		}
		return nil, fmt.Errorf("json_parse: top-level is not an object")
	}

	return orchkit.Output{"value": parsed}, nil
}
