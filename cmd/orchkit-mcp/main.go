package main

import (
	"context"
	"log"
	"os"

	"github.com/shaiksadikjanu-cmd/orchkit/ai"
	"github.com/shaiksadikjanu-cmd/orchkit/nodes"
)

// orchkit-mcp is the MCP server binary.
// Claude Desktop launches this process and communicates via stdin/stdout.
//
// Build:
//   go build -o orchkit-mcp ./cmd/orchkit-mcp
//
// Claude Desktop config (~/.config/claude/claude_desktop_config.json):
//
//   {
//     "mcpServers": {
//       "github.com/shaiksadikjanu-cmd/orchkit": {
//         "command": "/home/YOUR_USER/orchkit/orchkit-mcp"
//       }
//     }
//   }

func main() {
	// All nodes registered here become tools in Claude Desktop.
	// Add or remove nodes freely — Claude sees exactly what you register.
	server := ai.NewMCPServer(
		nodes.NewHTTPGet(""),
		nodes.NewJSONParse(""),
		nodes.NewFSRead(""),
		nodes.NewFSWrite(""),
		nodes.NewShell(""),
		nodes.NewGroqLLM(os.Getenv("GROQ_API_KEY"), ""),
		nodes.NewGeminiLLM(os.Getenv("GEMINI_API_KEY"), ""),
	)

	ctx := context.Background()
	if err := server.Serve(ctx); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
