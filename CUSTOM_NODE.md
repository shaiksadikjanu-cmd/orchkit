# How to Add a Custom Node to orchkit

A node is the unit of work in orchkit. Every integration — HTTP, database,
AI, messaging — is a node. Adding your own takes 5 minutes.

## The contract

Every node implements 3 methods:

```go
type Node interface {
    Name()    string          // unique identifier e.g. "my_node"
    Schema()  orchkit.Schema  // describes inputs for humans and AI
    Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error)
}
```

## Step 1 — Create the file

```bash
cat > ~/orchkit/nodes/my_node.go << 'GOEOF'
package nodes

import (
    "context"
    "fmt"
    "orchkit"
)

type MyNode struct {
    DefaultValue string
}

func NewMyNode(defaultValue string) *MyNode {
    return &MyNode{DefaultValue: defaultValue}
}

func (n *MyNode) Name() string { return "my_node" }

func (n *MyNode) Schema() orchkit.Schema {
    return orchkit.Schema{
        Description: "Does something useful. Describe it here.",
        Params: map[string]any{
            "input": map[string]any{
                "type": "string",
                "desc": "The input value to process.",
            },
        },
    }
}

func (n *MyNode) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
    value, ok := in["input"].(string)
    if !ok || value == "" {
        value = n.DefaultValue
    }
    if value == "" {
        return nil, fmt.Errorf("my_node: input is required")
    }

    // Your logic here.
    result := "processed: " + value

    return orchkit.Output{
        "result": result,
        "length": len(result),
    }, nil
}
'GOEOF'
```

## Step 2 — Use it in a flow

```go
import "orchkit"
import "orchkit/nodes"

flow := orchkit.NewFlow().
    Step("process", nodes.NewMyNode("default"))

state, err := orchkit.Run(ctx, flow, orchkit.NewMemStore())
```

## Step 3 — Use it in YAML

Register it in `cmd/orchkit/registry.go`:

```go
r.Register("my_node", func() orchkit.Node {
    return nodes.NewMyNode(os.Getenv("MY_NODE_DEFAULT"))
})
```

Then in a flow file:

```yaml
name: my-flow
steps:
  - id: process
    node: my_node
    input:
      input: "hello world"
```

Run it:

```bash
orchkit run my-flow.yaml
```

## Step 4 — Use it as an AI tool

No changes needed. Any node registered in the CLI is automatically
available as an AI tool in the agent loop and MCP server.

```go
result, err := ai.Run(ctx, ai.AgentConfig{
    GroqAPIKey: os.Getenv("GROQ_API_KEY"),
    Nodes: []orchkit.Node{
        nodes.NewMyNode("default"),
        nodes.NewHTTPGet(""),
    },
}, "Use my_node to process the string 'hello world'")
```

## Rules for good nodes

1. Import only `orchkit` and stdlib — never another node
2. One file, one struct, one concern
3. If no input provided, fall back to constructor value
4. Return errors with the node name as prefix: `fmt.Errorf("my_node: %w", err)`
5. Non-fatal results (e.g. HTTP 404) go in output, not errors
6. Keep `Execute` under 80 lines — if longer, the node is doing too much

## Examples to learn from

- Simple: `nodes/delay.go` — minimal, clean
- HTTP: `nodes/http.go` — shows timeout, context, fallback
- Database: `nodes/sqlite.go` — shows query vs exec pattern
- AI: `nodes/llm_groq.go` — shows external API call pattern
