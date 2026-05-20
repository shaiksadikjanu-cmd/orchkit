package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/shaiksadikjanu-cmd/orchkit"
	"github.com/shaiksadikjanu-cmd/orchkit/ai"
	"github.com/shaiksadikjanu-cmd/orchkit/nodes"
)

func main() {
	ctx := context.Background()

	// Build a flow: fetch a public JSON endpoint, parse it, surface a field.
	flow := orchkit.NewFlow().
		Step("fetch", nodes.NewHTTPGet("https://httpbin.org/json")).
		Step("parse", nodes.NewJSONParse("slideshow"))

	// Run it against an in-memory store.
	state, err := orchkit.Run(ctx, flow, orchkit.NewMemStore())
	if err != nil {
		log.Fatalf("flow failed: %v", err)
	}

	pretty, _ := json.MarshalIndent(state, "", "  ")
	fmt.Println("---- final state ----")
	fmt.Println(string(pretty))

	// And here's the same nodes exposed as AI tools — zero changes to them.
	tools := ai.Tools(
		nodes.NewHTTPGet(""),
		nodes.NewJSONParse(""),
	)
	toolsJSON, _ := ai.JSON(tools)
	fmt.Println("\n---- AI tool schema (hand this to Claude) ----")
	fmt.Println(toolsJSON)
}
