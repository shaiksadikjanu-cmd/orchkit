package orchkit

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadYAML reads a flow definition from a YAML file.
func LoadYAML(path string) (YAMLFlow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return YAMLFlow{}, fmt.Errorf("orchkit: read %s: %w", path, err)
	}
	var flow YAMLFlow
	if err := yaml.Unmarshal(data, &flow); err != nil {
		return YAMLFlow{}, fmt.Errorf("orchkit: parse %s: %w", path, err)
	}
	if flow.Name == "" {
		flow.Name = path
	}
	return flow, nil
}

// MustLoadYAML loads a YAML flow or panics.
func MustLoadYAML(path string) YAMLFlow {
	flow, err := LoadYAML(path)
	if err != nil {
		panic(err)
	}
	return flow
}

// InterpolateStep resolves ${step.field} and ${ENV_VAR} references in a
// step's input values using the current flow state snapshot.
//
// Syntax:
//   ${step_id.field}     — value from a previous step's output
//   ${ENV_VAR}           — environment variable
//   ${step_id}           — entire output of a step (JSON-encoded)
//
// Example YAML:
//   - id: summarize
//     node: llm_groq
//     input:
//       prompt: "Summarize: ${fetch.items}"
func InterpolateStep(input map[string]any, state map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	result := make(map[string]any, len(input))
	for k, v := range input {
		switch val := v.(type) {
		case string:
			result[k] = interpolateString(val, state)
		default:
			result[k] = v
		}
	}
	return result
}

// interpolateString replaces ${ref} tokens in a string.
func interpolateString(s string, state map[string]any) string {
	if !strings.Contains(s, "${") {
		return os.ExpandEnv(s)
	}

	result := s
	for {
		start := strings.Index(result, "${")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		ref := result[start+2 : end]
		value := resolveRef(ref, state)
		result = result[:start] + value + result[end+1:]
	}
	return result
}

// resolveRef resolves a single reference like "step.field" or "ENV_VAR".
func resolveRef(ref string, state map[string]any) string {
	// Check environment first for ALL_CAPS names.
	if strings.ToUpper(ref) == ref {
		if v := os.Getenv(ref); v != "" {
			return v
		}
	}

	// Dot notation: step_id.field or step_id.nested.field
	parts := strings.SplitN(ref, ".", 2)
	stepID := parts[0]

	stepVal, ok := state[stepID]
	if !ok {
		// Try as env var fallback.
		return os.Getenv(ref)
	}

	if len(parts) == 1 {
		// Return entire step output as string.
		return fmt.Sprintf("%v", stepVal)
	}

	// Navigate into the step's output map.
	return navigateDot(stepVal, parts[1])
}

// navigateDot navigates nested maps using dot notation.
func navigateDot(v any, path string) string {
	parts := strings.SplitN(path, ".", 2)
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	val, ok := m[parts[0]]
	if !ok {
		return ""
	}
	if len(parts) == 1 {
		return fmt.Sprintf("%v", val)
	}
	return navigateDot(val, parts[1])
}
