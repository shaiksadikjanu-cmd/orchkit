# orchkit

A composable orchestration kernel in Go. The engine is not a monolith — it is a kit of small parts you import and wire yourself. Nodes are self-contained "arms" with high cohesion and low coupling. The same node can be called by a Go developer or by an AI agent.

## Principles

1. Every line of code is a liability. Write the least code that makes the next change easy.
2. No global registries, no init() side effects. Wiring is always explicit, so the Go linker drops unused code.
3. A node imports only the kernel. Never another node, never an execution detail.
4. Interfaces appear only when there are two implementations. Not before.
5. One concept per file. Files stay small enough to read in one sitting.

## Layout

    orchkit/
    ├── orchkit.go              Kernel: Node, Flow, Run, Store, MemStore
    ├── nodes/
    │   ├── http.go             HTTPGet node
    │   └── transform.go        JSONParse node
    ├── ai/
    │   └── tool.go             Node -> AI tool adapter
    └── examples/
        └── fetch-parse/main.go A runnable demo

## Run the demo

    cd examples/fetch-parse
    go run .

You'll see the flow's final state, then the same nodes printed as an AI tool schema.

## Adding a new node

Create one file under nodes/. Implement three methods: Name, Schema, Execute. Import only orchkit and stdlib. Done.

## Adding a new store

Create one file at the top level (boltstore.go, pgstore.go). Implement Get, Put, Snapshot. Pass it to orchkit.Run. Done.

## What's not here yet (and why)

- Retry / timeout / circuit-breaker wrappers — add when you actually hit a flaky node.
- Parallel / branch steps — add when a sequential flow stops being enough.
- Event bus, durable replay, distributed execution — add when one machine stops being enough.

Resist adding any of these speculatively.
