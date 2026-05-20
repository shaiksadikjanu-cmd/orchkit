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

// Data Pipeline
// -------------
// 1. Fetch live cryptocurrency prices from CoinGecko public API
// 2. Parse the JSON response
// 3. Store each coin in SQLite — sequentially (SQLite is single-writer)
// 4. Query the database for analysis
// 5. Generate a markdown report
// 6. Write report to file
//
// Run:
//   go run .
// No API keys needed.

func main() {
	ctx := context.Background()

	fmt.Println("◆ orchkit Data Pipeline")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("CoinGecko API → SQLite → Market Report")
	fmt.Println()

	dbPath := "/tmp/orchkit-crypto.db"
	reportPath := "/tmp/orchkit-crypto-report.md"
	os.Remove(dbPath)

	hooks := &orchkit.Hooks{
		OnStepStart: func(id string, in orchkit.Input) {
			fmt.Printf("  → %s\n", id)
		},
		OnStepEnd: func(id string, out orchkit.Output, err error, elapsed time.Duration) {
			if err != nil {
				fmt.Printf("  ✗ %s: %v\n", id, err)
			} else {
				fmt.Printf("  ✓ %s (%s)\n", id, elapsed.Round(time.Millisecond))
			}
		},
	}

	// ── Phase 1: Schema + Fetch ───────────────────────────────────────────

	fmt.Println("[Phase 1] Create schema + fetch prices")
	fmt.Println("────────────────────────────────────────")

	phase1 := orchkit.NewFlow().
		StepWith(orchkit.Step{
			ID: "create_schema",
			Node: nodes.NewSQLite(dbPath,
				`CREATE TABLE IF NOT EXISTS prices (
					id         TEXT PRIMARY KEY,
					symbol     TEXT,
					name       TEXT,
					price_usd  REAL,
					change_24h REAL,
					market_cap REAL,
					fetched_at TEXT
				)`),
		}).
		StepWith(orchkit.Step{
			ID: "fetch_prices",
			Node: nodes.NewHTTPGet(
				"https://api.coingecko.com/api/v3/coins/markets" +
					"?vs_currency=usd&order=market_cap_desc&per_page=10&page=1",
			),
			Timeout: 15 * time.Second,
		})

	phase1Store := orchkit.NewMemStore()
	phase1State, err := orchkit.Run(ctx, phase1, phase1Store,
		orchkit.RunOptions{Hooks: hooks})
	if err != nil {
		log.Fatalf("phase 1 failed: %v", err)
	}

	// Parse coins from response body.
	fetchResult, _ := phase1State["fetch_prices"].(map[string]any)
	body, _ := fetchResult["body"].(string)

	var coins []map[string]any
	if err := json.Unmarshal([]byte(body), &coins); err != nil {
		log.Fatalf("parse JSON failed: %v", err)
	}
	fmt.Printf("  ✓ parsed %d coins\n", len(coins))

	// ── Phase 2: Insert sequentially ─────────────────────────────────────

	fmt.Println("\n[Phase 2] Inserting coins into SQLite (sequential)")
	fmt.Println("────────────────────────────────────────")

	now := time.Now().Format(time.RFC3339)

	// Build one flow with all inserts as sequential steps.
	insertFlow := orchkit.NewFlow()
	for _, coin := range coins {
		id, _ := coin["id"].(string)
		symbol, _ := coin["symbol"].(string)
		name, _ := coin["name"].(string)
		price, _ := coin["current_price"].(float64)
		change, _ := coin["price_change_percentage_24h"].(float64)
		mcap, _ := coin["market_cap"].(float64)

		query := fmt.Sprintf(
			`INSERT OR REPLACE INTO prices VALUES ('%s','%s','%s',%.8f,%.4f,%.0f,'%s')`,
			id, symbol, name, price, change, mcap, now,
		)
		insertFlow.Step("insert_"+id, nodes.NewSQLite(dbPath, query))
	}

	_, err = orchkit.Run(ctx, insertFlow, orchkit.NewMemStore(),
		orchkit.RunOptions{Hooks: hooks})
	if err != nil {
		log.Fatalf("insert failed: %v", err)
	}

	// ── Phase 3: Query and analyze ────────────────────────────────────────

	fmt.Println("\n[Phase 3] Querying database")
	fmt.Println("────────────────────────────────────────")

	queryFlow := orchkit.NewFlow().
		StepWith(orchkit.Step{
			ID:   "all_coins",
			Node: nodes.NewSQLite(dbPath, "SELECT * FROM prices ORDER BY market_cap DESC"),
		}).
		StepWith(orchkit.Step{
			ID:   "top_gainer",
			Node: nodes.NewSQLite(dbPath, "SELECT name, symbol, change_24h FROM prices ORDER BY change_24h DESC LIMIT 1"),
		}).
		StepWith(orchkit.Step{
			ID:   "top_loser",
			Node: nodes.NewSQLite(dbPath, "SELECT name, symbol, change_24h FROM prices ORDER BY change_24h ASC LIMIT 1"),
		}).
		StepWith(orchkit.Step{
			ID:   "avg_change",
			Node: nodes.NewSQLite(dbPath, "SELECT ROUND(AVG(change_24h),4) as avg_change FROM prices"),
		}).
		StepWith(orchkit.Step{
			ID:   "total_mcap",
			Node: nodes.NewSQLite(dbPath, "SELECT ROUND(SUM(market_cap)/1e9,2) as total_mcap_billion FROM prices"),
		})

	queryStore := orchkit.NewMemStore()
	queryState, err := orchkit.Run(ctx, queryFlow, queryStore,
		orchkit.RunOptions{Hooks: hooks})
	if err != nil {
		log.Fatalf("query failed: %v", err)
	}

	// ── Phase 4: Build and write report ──────────────────────────────────

	fmt.Println("\n[Phase 4] Building report")
	fmt.Println("────────────────────────────────────────")

	allCoins := extractRows(queryState, "all_coins")
	topGainer := extractFirstRow(queryState, "top_gainer")
	topLoser := extractFirstRow(queryState, "top_loser")
	avgChange := extractFirstRow(queryState, "avg_change")
	totalMcap := extractFirstRow(queryState, "total_mcap")

	var report strings.Builder
	report.WriteString("# ◆ orchkit Crypto Market Report\n\n")
	report.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	report.WriteString("---\n\n## Market Summary\n\n")

	if avgChange != nil {
		report.WriteString(fmt.Sprintf("- **Avg 24h Change:** %.4f%%\n", toFloat(avgChange["avg_change"])))
	}
	if totalMcap != nil {
		report.WriteString(fmt.Sprintf("- **Total Market Cap:** $%.2fB\n", toFloat(totalMcap["total_mcap_billion"])))
	}
	if topGainer != nil {
		report.WriteString(fmt.Sprintf("- **Top Gainer:** %s (%s) +%.2f%%\n",
			topGainer["name"], strings.ToUpper(fmt.Sprint(topGainer["symbol"])),
			toFloat(topGainer["change_24h"])))
	}
	if topLoser != nil {
		report.WriteString(fmt.Sprintf("- **Top Loser:** %s (%s) %.2f%%\n",
			topLoser["name"], strings.ToUpper(fmt.Sprint(topLoser["symbol"])),
			toFloat(topLoser["change_24h"])))
	}

	report.WriteString("\n---\n\n## Top 10 by Market Cap\n\n")
	report.WriteString("| # | Coin | Symbol | Price (USD) | 24h Change | Market Cap |\n")
	report.WriteString("|---|------|--------|-------------|------------|------------|\n")

	for i, row := range allCoins {
		change := toFloat(row["change_24h"])
		changeStr := fmt.Sprintf("%.2f%%", change)
		if change > 0 {
			changeStr = "+" + changeStr
		}
		report.WriteString(fmt.Sprintf("| %d | %s | %s | $%.2f | %s | $%.0fB |\n",
			i+1,
			row["name"],
			strings.ToUpper(fmt.Sprint(row["symbol"])),
			toFloat(row["price_usd"]),
			changeStr,
			toFloat(row["market_cap"])/1e9,
		))
	}
	report.WriteString("\n---\n*Generated by orchkit data pipeline*\n")

	reportContent := report.String()

	writeFlow := orchkit.NewFlow().Step("write_report", nodes.NewFSWrite(reportPath))
	writeStore := orchkit.NewMemStore()
	writeStore.Put(ctx, "content", reportContent)
	_, err = orchkit.Run(ctx, writeFlow, writeStore, orchkit.RunOptions{Hooks: hooks})
	if err != nil {
		log.Fatalf("write failed: %v", err)
	}

	// ── Final output ──────────────────────────────────────────────────────

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("◆ PIPELINE COMPLETE")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Coins stored  : %d\n", len(coins))
	fmt.Printf("Database      : %s\n", dbPath)
	fmt.Printf("Report        : %s\n\n", reportPath)
	fmt.Println(reportContent)

	// Verify DB directly with a shell query.
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("DB verification (sqlite3 direct query):")
	verifyFlow := orchkit.NewFlow().
		Step("verify", nodes.NewShell(
			fmt.Sprintf("sqlite3 %s 'SELECT name, printf(\"$%%.2f\", price_usd), printf(\"%%.2f%%\", change_24h) FROM prices ORDER BY market_cap DESC'", dbPath),
		))
	verifyState, err := orchkit.Run(ctx, verifyFlow, orchkit.NewMemStore())
	if err == nil {
		if v, ok := verifyState["verify"].(map[string]any); ok {
			fmt.Println(v["stdout"])
		}
	}
}

func extractRows(state map[string]any, stepID string) []map[string]any {
	stepOut, ok := state[stepID].(map[string]any)
	if !ok {
		return nil
	}
	rows, ok := stepOut["rows"].([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			result = append(result, m)
		}
	}
	return result
}

func extractFirstRow(state map[string]any, stepID string) map[string]any {
	rows := extractRows(state, stepID)
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	}
	return 0
}
