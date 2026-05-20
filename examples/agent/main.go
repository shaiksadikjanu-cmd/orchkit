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
	// Set whichever keys you have. Agent uses whichever is non-empty.
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	groqKey      := os.Getenv("GROQ_API_KEY")
	geminiKey    := os.Getenv("GEMINI_API_KEY")

	// Pick the first available key + matching LLM node.
	// Preference: Groq first (free + fast), then Gemini, then Anthropic.
	var llmNode orchkit.Node
	var providerName string

	switch {
	case groqKey != "":
		llmNode = nodes.NewGroqLLM(groqKey, "llama-3.3-70b-versatile")
		providerName = "Groq (llama-3.3-70b-versatile)"
	case geminiKey != "":
		llmNode = nodes.NewGeminiLLM(geminiKey, "gemini-2.0-flash")
		providerName = "Gemini (gemini-2.0-flash)"
	case anthropicKey != "":
		llmNode = nodes.NewLLM(anthropicKey, "claude-sonnet-4-20250514")
		providerName = "Anthropic (claude-sonnet-4-20250514)"
	default:
		log.Fatal("set at least one of: GROQ_API_KEY, GEMINI_API_KEY, ANTHROPIC_API_KEY")
	}

	fmt.Printf("Using provider: %s\n\n", providerName)

	ctx := context.Background()

	// The agent loop always uses Anthropic for tool-use orchestration.
	// The llmNode above is available as a tool the agent can call.
	agentKey := anthropicKey
	if agentKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required for the agent loop (tool-use). Set GROQ_API_KEY or GEMINI_API_KEY for the LLM node.")
	}

	result, err := ai.Run(ctx, ai.AgentConfig{
		APIKey: agentKey,
		System: "You are a helpful agent. Use the tools available to complete tasks. Be concise.",
		Nodes: []orchkit.Node{
			nodes.NewHTTPGet(""),
			nodes.NewJSONParse(""),
			nodes.NewFSWrite(""),
			llmNode, // whichever LLM the user has a key for
		},
	}, "Fetch https://httpbin.org/json, parse the slideshow title, then write it to /tmp/orchkit-result.txt")

	if err != nil {
		log.Fatalf("agent failed: %v", err)
	}

	fmt.Println("---- Claude's final response ----")
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

	content, err := os.ReadFile("/tmp/orchkit-result.txt")
	if err == nil {
		fmt.Println("\n---- Written file ----")
		fmt.Println(string(content))
	}
}
