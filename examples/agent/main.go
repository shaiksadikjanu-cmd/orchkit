package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"orchkit"
	"orchkit/ai"
	"orchkit/nodes"
)

func main() {
	groqKey   := os.Getenv("GROQ_API_KEY")
	geminiKey := os.Getenv("GEMINI_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")

	if groqKey == "" && geminiKey == "" && anthropicKey == "" {
		log.Fatal("set at least one of: GROQ_API_KEY, GEMINI_API_KEY, ANTHROPIC_API_KEY")
	}

	ctx := context.Background()

	result, err := ai.Run(ctx, ai.AgentConfig{
		GroqAPIKey:      groqKey,
		GeminiAPIKey:    geminiKey,
		AnthropicAPIKey: anthropicKey,
		System: "You are a helpful agent. Use the tools available to complete tasks. Be concise.",
		Nodes: []orchkit.Node{
			nodes.NewHTTPGet(""),
			nodes.NewJSONParse(""),
			nodes.NewFSWrite(""),
		},
	}, "Fetch https://httpbin.org/json, parse the slideshow title, then write it to /tmp/orchkit-result.txt")

	if err != nil {
		log.Fatalf("agent failed: %v", err)
	}

	fmt.Println("---- Agent final response ----")
	fmt.Println(result.Text)
	fmt.Printf("\nCompleted in %d turn(s)\n", result.Turns)

	fmt.Println("\n---- Tool calls made ----")
	for i, call := range result.Calls {
		fmt.Printf("[%d] %s\n", i+1, call.Node)
		inp, _ := json.MarshalIndent(call.Input, "    ", "  ")
		fmt.Printf("    input:  %s\n", inp)
		if call.Err != nil {
			fmt.Printf("    error:  %v\n", call.Err)
		} else {
			out, _ := json.MarshalIndent(call.Output, "    ", "  ")
			fmt.Printf("    output: %s\n", out)
		}
	}

	content, _ := os.ReadFile("/tmp/orchkit-result.txt")
	if content != nil {
		fmt.Println("\n---- Written file ----")
		fmt.Println(string(content))
	}
}
