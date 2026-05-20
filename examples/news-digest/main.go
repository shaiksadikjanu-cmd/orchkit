package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"orchkit"
	"orchkit/nodes"
)

// News Digest Agent
// -----------------
// 1. Fetch top 5 items from Hacker News RSS
// 2. For each item — summarize with Groq LLM
// 3. Build a digest from all summaries using template
// 4. Write the digest to a file
// 5. Print the result
//
// Run:
//   export GROQ_API_KEY=your_key
//   go run .

func main() {
	groqKey := os.Getenv("GROQ_API_KEY")
	if groqKey == "" {
		log.Fatal("GROQ_API_KEY is required — get one free at console.groq.com")
	}

	ctx := context.Background()
	store := orchkit.NewMemStore()

	fmt.Println("◆ orchkit News Digest Agent")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Step 1: Fetch Hacker News RSS feed — top 5 items only.
	fmt.Println("\n[1/4] Fetching Hacker News feed...")
	rssNode := nodes.NewRSS("https://hnrss.org/frontpage").WithLimit(5)
	rssOut, err := rssNode.Execute(ctx, nil)
	if err != nil {
		log.Fatalf("RSS fetch failed: %v", err)
	}

	items, ok := rssOut["items"].([]any)
	if !ok || len(items) == 0 {
		log.Fatal("No items found in feed")
	}
	fmt.Printf("    Found %d articles\n", len(items))

	// Step 2: Summarize each article with Groq LLM.
	fmt.Println("\n[2/4] Summarizing articles with Groq LLM...")
	llm := nodes.NewGroqLLM(groqKey, "llama-3.3-70b-versatile")

	type summary struct {
		Title   string
		Link    string
		Summary string
	}
	summaries := make([]summary, 0, len(items))

	hooks := &orchkit.Hooks{
		OnStepStart: func(id string, in orchkit.Input) {
			fmt.Printf("    → summarizing: %s\n", id)
		},
		OnStepEnd: func(id string, out orchkit.Output, err error, elapsed time.Duration) {
			if err != nil {
				fmt.Printf("    ✗ %s failed: %v\n", id, err)
			} else {
				fmt.Printf("    ✓ %s done (%s)\n", id, elapsed.Round(time.Millisecond))
			}
		},
	}

	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		title, _ := m["title"].(string)
		link, _ := m["link"].(string)
		desc, _ := m["description"].(string)

		// Build a mini flow per article — shows flow composition at work.
		articleFlow := orchkit.NewFlow().
			StepWith(orchkit.Step{
				ID:   fmt.Sprintf("article_%d", i+1),
				Node: llm,
				In:   nil,
			})

		articleStore := orchkit.NewMemStore()
		prompt := fmt.Sprintf(
			"Summarize this Hacker News article in 2 sentences. Be direct and technical.\n\nTitle: %s\nDescription: %s",
			title, stripHTML(desc),
		)
		articleStore.Put(ctx, "prompt", prompt)

		articleState, err := orchkit.Run(ctx, articleFlow, articleStore,
			orchkit.RunOptions{Hooks: hooks})

		sum := "[summary unavailable]"
		if err == nil {
			if stepOut, ok := articleState[fmt.Sprintf("article_%d", i+1)].(map[string]any); ok {
				if t, ok := stepOut["text"].(string); ok && t != "" {
					sum = strings.TrimSpace(t)
				}
			}
		}

		summaries = append(summaries, summary{
			Title:   title,
			Link:    link,
			Summary: sum,
		})

		// Rate limit — Groq free tier.
		if i < len(items)-1 {
			time.Sleep(2 * time.Second)
		}
	}

	// Step 3: Build the digest using template node.
	fmt.Println("\n[3/4] Building digest...")

	var digestBuilder strings.Builder
	digestBuilder.WriteString(fmt.Sprintf("# Hacker News Digest\n"))
	digestBuilder.WriteString(fmt.Sprintf("Generated: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	digestBuilder.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	for i, s := range summaries {
		digestBuilder.WriteString(fmt.Sprintf("## %d. %s\n", i+1, s.Title))
		digestBuilder.WriteString(fmt.Sprintf("🔗 %s\n\n", s.Link))
		digestBuilder.WriteString(fmt.Sprintf("%s\n\n", s.Summary))
		digestBuilder.WriteString("────────────────────────────────────────\n\n")
	}

	digestContent := digestBuilder.String()

	// Step 4: Write digest to file using fs_write node.
	fmt.Println("[4/4] Writing digest to file...")
	outputPath := "/tmp/orchkit-news-digest.md"

	writeFlow := orchkit.NewFlow().
		Step("write", nodes.NewFSWrite(outputPath))

	writeStore := orchkit.NewMemStore()
	writeStore.Put(ctx, "content", digestContent)

	_, err = orchkit.Run(ctx, writeFlow, writeStore)
	if err != nil {
		log.Fatalf("Write failed: %v", err)
	}

	// Save full state to main store for inspection.
	store.Put(ctx, "item_count", len(summaries))
	store.Put(ctx, "output_path", outputPath)
	store.Put(ctx, "generated_at", time.Now().Format(time.RFC3339))

	// Print results.
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("◆ DIGEST COMPLETE")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Articles processed : %d\n", len(summaries))
	fmt.Printf("Output file        : %s\n", outputPath)
	fmt.Println()
	fmt.Println(digestContent)

	// Show final flow state.
	snap, _ := store.Snapshot(ctx)
	stateJSON, _ := json.MarshalIndent(snap, "", "  ")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("Flow state:")
	fmt.Println(string(stateJSON))
}

// stripHTML removes basic HTML tags from a string.
func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}
