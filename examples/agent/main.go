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
	groqKey      := os.Getenv("GROQ_API_KEY")
	geminiKey    := os.Getenv("GEMINI_API_KEY")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")

	if groqKey == "" && geminiKey == "" && anthropicKey == "" {
		log.Fatal("set at least one of: GROQ_API_KEY, GEMINI_API_KEY, ANTHROPIC_API_KEY")
	}

	ctx := context.Background()

	result, err := ai.Run(ctx, ai.AgentConfig{
		GroqAPIKey:      groqKey,
		GeminiAPIKey:    geminiKey,
		AnthropicAPIKey: anthropicKey,
		MaxTokens:       512,
		System: `You are a precise tool-calling agent. Rules:
- Always pass the exact string value from a previous tool's output, never modify it.
- The http_get tool returns a JSON object with a "body" key containing a raw JSON string.
- Pass that exact "body" string value to json_parse.
- Never wrap values in b'...' or any other notation.`,
		Nodes: []orchkit.Node{
			nodes.NewHTTPGet(""),
			nodes.NewJSONParse(""),
			nodes.NewFSWrite(""),
		},
	}, `Step 1: Call http_get with url="https://httpbin.org/json".
Step 2: Take the exact string value of the "body" field from step 1's output. Pass it as "body" to json_parse with field="slideshow".
Step 3: Take the "title" field from the parsed slideshow object. Write it as a string to /tmp/orchkit-result.txt using fs_write.`)

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
