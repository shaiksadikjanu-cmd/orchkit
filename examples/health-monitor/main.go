package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"orchkit"
	"orchkit/nodes"
)

// Health Monitor
// --------------
// Monitors a list of URLs every 30 seconds.
// If any endpoint fails (non-200 or timeout):
//   → logs the failure
//   → sends a Telegram alert (if TELEGRAM_TOKEN + TELEGRAM_CHAT_ID set)
//   → continues monitoring
//
// Run:
//   export TELEGRAM_TOKEN=your_bot_token      (optional)
//   export TELEGRAM_CHAT_ID=your_chat_id      (optional)
//   go run .

// Endpoint defines one URL to monitor.
type Endpoint struct {
	Name string
	URL  string
}

// Result holds one check result.
type Result struct {
	Name    string
	URL     string
	Status  int
	OK      bool
	Latency time.Duration
	Error   string
}

var endpoints = []Endpoint{
	{Name: "Hacker News", URL: "https://news.ycombinator.com"},
	{Name: "HTTPBin",     URL: "https://httpbin.org/status/200"},
	{Name: "GitHub",      URL: "https://github.com"},
	{Name: "Broken Site", URL: "https://httpbin.org/status/503"}, // always fails — proves alerting
}

const checkInterval = 30 * time.Second

func main() {
	telegramToken  := os.Getenv("TELEGRAM_TOKEN")
	telegramChatID := os.Getenv("TELEGRAM_CHAT_ID")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("◆ orchkit Health Monitor")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Monitoring %d endpoints every %s\n", len(endpoints), checkInterval)
	fmt.Println("Press Ctrl+C to stop.")
	if telegramToken != "" {
		fmt.Println("Telegram alerts: enabled")
	} else {
		fmt.Println("Telegram alerts: disabled (set TELEGRAM_TOKEN + TELEGRAM_CHAT_ID to enable)")
	}
	fmt.Println()

	// Store tracks run history across loops.
	store, _ := orchkit.NewBoltStore("/tmp/orchkit-health.db")
	defer store.Close()

	round := 0

	// Main monitoring loop — runs until Ctrl+C.
	loopNode := orchkit.NewLoopNode(
		&monitorNode{
			endpoints:      endpoints,
			telegramToken:  telegramToken,
			telegramChatID: telegramChatID,
			store:          store,
			getRound:       func() int { return round },
			incRound:       func() { round++ },
		},
		nil,        // until=nil means loop forever
		999999,     // effectively infinite
	).WithDelay(checkInterval)

	_, err := loopNode.Execute(ctx, nil)
	if err != nil && ctx.Err() == nil {
		log.Fatalf("monitor error: %v", err)
	}

	fmt.Println("\n◆ Health monitor stopped gracefully.")
}

// monitorNode implements orchkit.Node — one full check cycle.
type monitorNode struct {
	endpoints      []Endpoint
	telegramToken  string
	telegramChatID string
	store          orchkit.Store
	getRound       func() int
	incRound       func()
}

func (m *monitorNode) Name() string           { return "health_check" }
func (m *monitorNode) Schema() orchkit.Schema { return orchkit.Schema{} }

func (m *monitorNode) Execute(ctx context.Context, _ orchkit.Input) (orchkit.Output, error) {
	m.incRound()
	round := m.getRound()
	now := time.Now()

	fmt.Printf("\n[Round %d] %s\n", round, now.Format("2006-01-02 15:04:05"))
	fmt.Println("────────────────────────────────────────")

	// Check all endpoints in parallel using orchkit's Parallel engine.
	results := make([]Result, len(m.endpoints))

	// Build a parallel flow — one http_get step per endpoint.
	steps := make([]orchkit.Step, len(m.endpoints))
	stepStores := make([]*orchkit.MemStore, len(m.endpoints))

	for i, ep := range m.endpoints {
		stepStores[i] = orchkit.NewMemStore()
		steps[i] = orchkit.Step{
			ID:      ep.Name,
			Node:    nodes.NewHTTPGet(ep.URL),
			Timeout: 10 * time.Second,
		}
	}

	// Run all checks in parallel.
	parallelFlow := orchkit.NewFlow().Parallel(steps...)
	parallelStore := orchkit.NewMemStore()

	start := time.Now()
	state, err := orchkit.Run(ctx, parallelFlow, parallelStore)
	totalLatency := time.Since(start)

	if err != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Collect results.
	failures := []Result{}
	for i, ep := range m.endpoints {
		result := Result{Name: ep.Name, URL: ep.URL}

		if stepOut, ok := state[ep.Name].(map[string]any); ok {
			status, _ := stepOut["status"].(int)
			result.Status = status
			result.OK = status >= 200 && status < 400
			result.Latency = totalLatency / time.Duration(len(m.endpoints))
		} else {
			result.OK = false
			result.Error = "no response"
			if err != nil {
				result.Error = err.Error()
			}
		}

		results[i] = result
		if !result.OK {
			failures = append(failures, result)
		}
	}

	// Print results table.
	fmt.Printf("%-20s %-8s %-8s %s\n", "ENDPOINT", "STATUS", "OK", "LATENCY")
	fmt.Println("────────────────────────────────────────")
	for _, r := range results {
		status := fmt.Sprintf("%d", r.Status)
		if r.Status == 0 {
			status = "error"
		}
		ok := "✓"
		if !r.OK {
			ok = "✗"
		}
		fmt.Printf("%-20s %-8s %-8s %s\n",
			r.Name, status, ok, r.Latency.Round(time.Millisecond))
	}

	// Save round results to BoltStore.
	ctx2 := context.Background()
	m.store.Put(ctx2, fmt.Sprintf("round_%d", round), map[string]any{
		"time":      now.Format(time.RFC3339),
		"results":   len(results),
		"failures":  len(failures),
		"latency_ms": totalLatency.Milliseconds(),
	})

	// Alert on failures — conditional: only if telegram configured.
	if len(failures) > 0 {
		fmt.Printf("\n⚠ %d endpoint(s) failing:\n", len(failures))
		for _, f := range failures {
			fmt.Printf("  ✗ %s — status %d\n", f.Name, f.Status)
		}

		if m.telegramToken != "" && m.telegramChatID != "" {
			alertFlow := orchkit.NewFlow().
				StepWith(orchkit.Step{
					ID:   "alert",
					Node: nodes.NewTelegram(m.telegramToken, m.telegramChatID),
					When: func(in orchkit.Input) bool {
						// Only alert — don't repeat same failure twice in a row.
						return true
					},
				})

			msg := fmt.Sprintf("🚨 orchkit Health Alert — Round %d\n\n", round)
			for _, f := range failures {
				msg += fmt.Sprintf("❌ %s\n   URL: %s\n   Status: %d\n\n", f.Name, f.URL, f.Status)
			}
			msg += fmt.Sprintf("Time: %s", now.Format("2006-01-02 15:04:05"))

			alertStore := orchkit.NewMemStore()
			alertStore.Put(ctx2, "text", msg)

			_, alertErr := orchkit.Run(ctx2, alertFlow, alertStore)
			if alertErr != nil {
				fmt.Printf("  Telegram alert failed: %v\n", alertErr)
			} else {
				fmt.Println("  Telegram alert sent ✓")
			}
		} else {
			fmt.Println("  (Telegram not configured — set TELEGRAM_TOKEN + TELEGRAM_CHAT_ID)")
		}
	} else {
		fmt.Println("\n✓ All endpoints healthy")
	}

	fmt.Printf("\nNext check in %s...\n", checkInterval)
	return orchkit.Output{
		"round":    round,
		"results":  len(results),
		"failures": len(failures),
	}, nil
}
