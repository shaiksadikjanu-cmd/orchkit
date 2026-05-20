package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"orchkit"
)

const usage = `◆ orchkit — composable orchestration kernel

Usage:
  orchkit run <flow.yaml>          Run a flow from a YAML file
  orchkit run <flow.yaml> --dry    Validate flow without executing
  orchkit nodes                    List all available nodes
  orchkit node <name>              Show a node's schema and example
  orchkit validate <flow.yaml>     Validate a flow file
  orchkit version                  Show version

Examples:
  orchkit run my-flow.yaml
  orchkit nodes
  orchkit node http_get
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(0)
	}

	registry := buildRegistry()

	switch os.Args[1] {
	case "run":
		if len(os.Args) < 3 {
			fmt.Println("Usage: orchkit run <flow.yaml>")
			os.Exit(1)
		}
		cmdRun(os.Args[2], registry, contains(os.Args, "--dry"))
	case "nodes":
		cmdNodes(registry)
	case "node":
		if len(os.Args) < 3 {
			fmt.Println("Usage: orchkit node <name>")
			os.Exit(1)
		}
		cmdNode(os.Args[2], registry)
	case "validate":
		if len(os.Args) < 3 {
			fmt.Println("Usage: orchkit validate <flow.yaml>")
			os.Exit(1)
		}
		cmdValidate(os.Args[2], registry)
	case "version":
		fmt.Println("orchkit v0.1.0")
	default:
		fmt.Printf("unknown command: %s\n\n", os.Args[1])
		fmt.Print(usage)
		os.Exit(1)
	}
}

func cmdRun(path string, registry *orchkit.Registry, dryRun bool) {
	flow, err := orchkit.LoadYAML(path)
	if err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("◆ orchkit run: %s\n", flow.Name)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	reg := registry.Build()
	for _, step := range flow.Steps {
		if _, ok := reg[step.Node]; !ok {
			fmt.Printf("error: step %q uses unknown node %q\n", step.ID, step.Node)
			fmt.Println("run 'orchkit nodes' to see available nodes")
			os.Exit(1)
		}
	}

	if dryRun {
		fmt.Printf("✓ flow is valid (%d steps)\n", len(flow.Steps))
		for _, s := range flow.Steps {
			fmt.Printf("  step %-20s → node: %s\n", s.ID, s.Node)
		}
		fmt.Println("\n(dry run — not executed)")
		return
	}

	hooks := &orchkit.Hooks{
		OnStepStart: func(id string, in orchkit.Input) {
			fmt.Printf("  → %-20s", id)
		},
		OnStepEnd: func(id string, out orchkit.Output, err error, elapsed time.Duration) {
			if err != nil {
				fmt.Printf(" ✗ %v\n", err)
			} else {
				fmt.Printf(" ✓ %s\n", elapsed.Round(time.Millisecond))
			}
		},
		OnFlowEnd: func(state map[string]any, err error) {
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			if err != nil {
				fmt.Printf("✗ flow failed: %v\n", err)
			} else {
				fmt.Println("✓ flow complete")
			}
		},
	}

	store := orchkit.NewMemStore()
	ctx := context.Background()
	for _, step := range flow.Steps {
		for k, v := range step.Input {
			if s, ok := v.(string); ok {
				step.Input[k] = expandEnv(s)
			}
			store.Put(ctx, k, step.Input[k])
		}
	}

	state, err := orchkit.RunYAML(ctx, flow, reg, store,
		orchkit.RunOptions{Hooks: hooks})
	if err != nil {
		fmt.Printf("\nerror: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nFinal state:")
	out, _ := json.MarshalIndent(state, "", "  ")
	fmt.Println(string(out))
}

func cmdNodes(registry *orchkit.Registry) {
	fmt.Println("◆ orchkit — available nodes")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	reg := registry.Build()
	names := registry.Names()
	sort.Strings(names)

	catMap := map[string]string{
		"http_get": "HTTP/Web", "http_post": "HTTP/Web", "webhook": "HTTP/Web",
		"json_parse": "Data", "json_build": "Data", "csv_read": "Data",
		"xml": "Data", "template": "Data", "markdown": "Data", "rss": "Data",
		"sqlite": "Database", "postgres": "Database", "mysql": "Database",
		"mongodb": "Database", "redis": "Database",
		"slack": "Messaging", "discord": "Messaging", "telegram": "Messaging",
		"twilio": "Messaging", "smtp": "Messaging",
		"s3": "Cloud", "kafka": "Cloud",
		"jwt": "Auth", "ssh": "Auth",
		"github": "Developer", "jira": "Developer", "linear": "Developer",
		"hubspot": "CRM", "salesforce": "CRM", "airtable": "CRM",
		"google_sheets": "Productivity", "gmail": "Productivity",
		"notion": "Productivity", "cron": "Productivity",
		"llm": "AI/LLM", "llm_groq": "AI/LLM",
		"llm_gemini": "AI/LLM", "openai": "AI/LLM",
		"delay": "Utilities", "env": "Utilities", "shell": "Utilities",
		"fs_read": "Utilities", "fs_write": "Utilities",
	}

	categories := map[string][]string{}
	for _, name := range names {
		cat := catMap[name]
		if cat == "" {
			cat = "Utilities"
		}
		categories[cat] = append(categories[cat], name)
	}

	catOrder := []string{
		"HTTP/Web", "Data", "Database", "Messaging",
		"Cloud", "Auth", "Developer", "CRM",
		"Productivity", "AI/LLM", "Utilities",
	}

	total := 0
	for _, cat := range catOrder {
		nodeNames := categories[cat]
		if len(nodeNames) == 0 {
			continue
		}
		sort.Strings(nodeNames)
		fmt.Printf("\n%s:\n", cat)
		for _, name := range nodeNames {
			node := reg[name]
			desc := ""
			if node != nil {
				desc = node.Schema().Description
				if len(desc) > 60 {
					desc = desc[:60] + "..."
				}
			}
			fmt.Printf("  %-20s %s\n", name, desc)
			total++
		}
	}
	fmt.Printf("\nTotal: %d nodes\n", total)
	fmt.Println("\nRun 'orchkit node <name>' for details and examples.")
}

func cmdNode(name string, registry *orchkit.Registry) {
	reg := registry.Build()
	node, ok := reg[name]
	if !ok {
		fmt.Printf("error: node %q not found\n", name)
		fmt.Println("Run 'orchkit nodes' to see all available nodes.")
		os.Exit(1)
	}

	schema := node.Schema()
	fmt.Printf("◆ node: %s\n", name)
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Description: %s\n\n", schema.Description)

	if len(schema.Params) > 0 {
		fmt.Println("Parameters:")
		params := make([]string, 0, len(schema.Params))
		for k := range schema.Params {
			params = append(params, k)
		}
		sort.Strings(params)
		for _, k := range params {
			p, _ := schema.Params[k].(map[string]any)
			typ, _ := p["type"].(string)
			desc, _ := p["desc"].(string)
			fmt.Printf("  %-15s %-10s %s\n", k, typ, desc)
		}
	}

	fmt.Printf("\nExample flow.yaml:\n")
	fmt.Printf("  name: use-%s\n  steps:\n    - id: step1\n      node: %s\n      input:\n", name, name)

	params := make([]string, 0, len(schema.Params))
	for k := range schema.Params {
		params = append(params, k)
	}
	sort.Strings(params)
	for _, k := range params {
		p, _ := schema.Params[k].(map[string]any)
		typ, _ := p["type"].(string)
		fmt.Printf("        %s: %s\n", k, exampleValue(typ))
	}

	fmt.Printf("\nGo usage:\n  node := nodes.New%s(...)\n  out, err := node.Execute(ctx, orchkit.Input{\n", toPascalCase(name))
	for _, k := range params {
		p, _ := schema.Params[k].(map[string]any)
		typ, _ := p["type"].(string)
		fmt.Printf("    %q: %s,\n", k, exampleValue(typ))
	}
	fmt.Println("  })")
}

func cmdValidate(path string, registry *orchkit.Registry) {
	flow, err := orchkit.LoadYAML(path)
	if err != nil {
		fmt.Printf("✗ invalid YAML: %v\n", err)
		os.Exit(1)
	}
	reg := registry.Build()
	valid := true
	for _, step := range flow.Steps {
		if _, ok := reg[step.Node]; !ok {
			fmt.Printf("✗ step %q: unknown node %q\n", step.ID, step.Node)
			valid = false
		}
	}
	if valid {
		fmt.Printf("✓ %s is valid (%d steps)\n", flow.Name, len(flow.Steps))
		for _, s := range flow.Steps {
			fmt.Printf("  %-20s → %s\n", s.ID, s.Node)
		}
	} else {
		os.Exit(1)
	}
}

func expandEnv(s string) string { return os.ExpandEnv(s) }

func exampleValue(typ string) string {
	switch typ {
	case "string":
		return `"your-value"`
	case "integer", "number":
		return "1"
	case "boolean":
		return "true"
	case "array":
		return `["item1", "item2"]`
	case "object":
		return `{key: value}`
	default:
		return `"value"`
	}
}

func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "")
}

func contains(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}
