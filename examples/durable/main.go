package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"orchkit"
	"orchkit/nodes"
)

// This example proves BoltStore survives restarts.
// Run it twice:
//   First run:  fetches URL, parses JSON, writes to DB. Prints state.
//   Second run: state already in DB — you'll see both old + new keys.

func main() {
	ctx := context.Background()

	store, err := orchkit.NewBoltStore("/tmp/orchkit-durable.db")
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Check what's already in the store from a previous run.
	existing, _ := store.Snapshot(ctx)
	if len(existing) > 0 {
		fmt.Println("=== State from previous run ===")
		pretty, _ := json.MarshalIndent(existing, "", "  ")
		fmt.Println(string(pretty))
		fmt.Println()
	} else {
		fmt.Println("=== No previous state found — first run ===")
	}

	// Run the flow — results accumulate in the same DB file.
	flow := orchkit.NewFlow().
		Step("fetch", nodes.NewHTTPGet("https://httpbin.org/json")).
		Step("parse", nodes.NewJSONParse("slideshow"))

	state, err := orchkit.Run(ctx, flow, store)
	if err != nil {
		log.Fatalf("flow: %v", err)
	}

	fmt.Println("=== State after this run ===")
	pretty, _ := json.MarshalIndent(state, "", "  ")
	fmt.Println(string(pretty))
	fmt.Println("\nRun again to see state persist across restarts.")
}
