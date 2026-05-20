package nodes

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Template renders a Go text/template string using the flow state as data.
// Any key in the input map is accessible as {{.key}} in the template.
//
// Example:
//
//	nodes.NewTemplate("Hello {{.name}}, you have {{.count}} messages.")
type Template struct {
	Tmpl string // if empty, taken from input["template"] at runtime
}

func NewTemplate(tmpl string) *Template {
	return &Template{Tmpl: tmpl}
}

func (t *Template) Name() string { return "template" }

func (t *Template) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Renders a Go text/template string using input values as data. Access values with {{.key}}.",
		Params: map[string]any{
			"template": map[string]any{"type": "string", "desc": "Template string. Falls back to constructor value."},
		},
	}
}

func (t *Template) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	tmplStr := t.Tmpl
	if v, ok := in["template"].(string); ok && v != "" {
		tmplStr = v
	}
	if tmplStr == "" {
		return nil, fmt.Errorf("template: no template string provided")
	}

	tmpl, err := template.New("github.com/shaiksadikjanu-cmd/orchkit").Parse(tmplStr)
	if err != nil {
		return nil, fmt.Errorf("template: parse: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, in); err != nil {
		return nil, fmt.Errorf("template: execute: %w", err)
	}

	return orchkit.Output{"text": buf.String()}, nil
}
