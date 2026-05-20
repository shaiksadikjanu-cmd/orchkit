package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"orchkit"
	"orchkit/nodes"
)

type Triage struct {
	Number   int
	Title    string
	Priority string
	Category string
	Action   string
	Comment  string
}

func main() {
	githubToken := os.Getenv("GITHUB_TOKEN")
	groqKey     := os.Getenv("GROQ_API_KEY")
	owner       := os.Getenv("GITHUB_OWNER")
	repo        := os.Getenv("GITHUB_REPO")
	dryRun      := os.Getenv("DRY_RUN") == "true"

	if githubToken == "" || groqKey == "" {
		log.Fatal("GITHUB_TOKEN and GROQ_API_KEY are required")
	}
	if owner == "" || repo == "" {
		log.Fatal("GITHUB_OWNER and GITHUB_REPO are required")
	}

	ctx := context.Background()

	fmt.Println("◆ orchkit GitHub Automation Agent")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Repository : %s/%s\n", owner, repo)
	fmt.Printf("Dry run    : %v\n", dryRun)
	fmt.Println()

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

	// ── Step 1: Fetch open issues ─────────────────────────────────────────

	fmt.Println("[1/4] Fetching open issues")
	fmt.Println("────────────────────────────────────────")

	github := nodes.NewGitHub(githubToken)
	issueStore := orchkit.NewMemStore()
	issueStore.Put(ctx, "action", "list_issues")
	issueStore.Put(ctx, "owner", owner)
	issueStore.Put(ctx, "repo", repo)

	issueFlow := orchkit.NewFlow().
		StepWith(orchkit.Step{ID: "list_issues", Node: github})

	issueState, err := orchkit.Run(ctx, issueFlow, issueStore,
		orchkit.RunOptions{Hooks: hooks})
	if err != nil {
		log.Fatalf("fetch issues failed: %v", err)
	}

	issueResult, _ := issueState["list_issues"].(map[string]any)
	result, _ := issueResult["result"].([]any)

	if len(result) == 0 {
		fmt.Println("No open issues found.")
		fmt.Printf("Create some at: https://github.com/%s/%s/issues/new\n", owner, repo)
		return
	}
	fmt.Printf("  Found %d open issue(s)\n\n", len(result))

	// ── Step 2: Triage each issue with AI ────────────────────────────────

	fmt.Println("[2/4] AI triage — analyzing each issue")
	fmt.Println("────────────────────────────────────────")

	llm := nodes.NewGroqLLM(groqKey, "llama-3.3-70b-versatile")
	triages := make([]Triage, 0, len(result))

	for i, item := range result {
		issue, ok := item.(map[string]any)
		if !ok {
			continue
		}
		number := int(toFloat(issue["number"]))
		title, _ := issue["title"].(string)
		body, _ := issue["body"].(string)
		if body == "" {
			body = "(no description provided)"
		}

		fmt.Printf("\n  Issue #%d: %s\n", number, title)

		prompt := fmt.Sprintf(`You are a senior software engineer triaging GitHub issues.

Analyze this issue and respond in EXACTLY this format (no extra text):
PRIORITY: [critical/high/medium/low]
CATEGORY: [bug/feature/docs/question/enhancement/security]
ACTION: [one sentence describing what needs to be done]
COMMENT: [2-3 sentence friendly comment to post on the issue explaining the triage result]

Issue Title: %s
Issue Body: %s`, title, truncate(body, 500))

		triageStore := orchkit.NewMemStore()
		triageStore.Put(ctx, "prompt", prompt)

		triageFlow := orchkit.NewFlow().
			StepWith(orchkit.Step{
				ID:   fmt.Sprintf("triage_%d", i+1),
				Node: llm,
			})

		triageState, err := orchkit.Run(ctx, triageFlow, triageStore,
			orchkit.RunOptions{Hooks: hooks})

		triage := Triage{Number: number, Title: title,
			Priority: "medium", Category: "feature", Action: "Review and assign"}

		if err == nil {
			if stepOut, ok := triageState[fmt.Sprintf("triage_%d", i+1)].(map[string]any); ok {
				if text, ok := stepOut["text"].(string); ok {
					triage = parseTriage(number, title, text)
				}
			}
		}

		triages = append(triages, triage)
		fmt.Printf("     Priority : %s\n", triage.Priority)
		fmt.Printf("     Category : %s\n", triage.Category)
		fmt.Printf("     Action   : %s\n", triage.Action)

		if i < len(result)-1 {
			time.Sleep(2 * time.Second)
		}
	}

	// ── Step 3: Post comments ─────────────────────────────────────────────

	fmt.Println("\n[3/4] Posting triage comments")
	fmt.Println("────────────────────────────────────────")

	if dryRun {
		fmt.Println("  DRY RUN — no comments will be posted")
	}

	for _, triage := range triages {
		comment := fmt.Sprintf(
			"🤖 **orchkit Triage Agent**\n\n"+
				"**Priority:** %s\n"+
				"**Category:** %s\n"+
				"**Suggested Action:** %s\n\n"+
				"%s\n\n"+
				"---\n*Triaged automatically by orchkit*",
			strings.ToUpper(triage.Priority),
			triage.Category,
			triage.Action,
			triage.Comment,
		)

		fmt.Printf("\n  Issue #%d — %s\n", triage.Number, triage.Title)

		if dryRun {
			fmt.Printf("  [DRY RUN] Comment:\n%s\n", indent(comment, "    "))
			continue
		}

		commentStore := orchkit.NewMemStore()
		commentStore.Put(ctx, "action", "create_comment")
		commentStore.Put(ctx, "owner", owner)
		commentStore.Put(ctx, "repo", repo)
		commentStore.Put(ctx, "number", float64(triage.Number))
		commentStore.Put(ctx, "body", comment)

		commentFlow := orchkit.NewFlow().
			StepWith(orchkit.Step{
				ID:   fmt.Sprintf("comment_%d", triage.Number),
				Node: github,
			})

		_, err := orchkit.Run(ctx, commentFlow, commentStore,
			orchkit.RunOptions{Hooks: hooks})
		if err != nil {
			fmt.Printf("  ✗ comment failed: %v\n", err)
		}
		time.Sleep(time.Second)
	}

	// ── Step 4: Write report ──────────────────────────────────────────────

	fmt.Println("\n[4/4] Writing triage report")
	fmt.Println("────────────────────────────────────────")

	var reportBuilder strings.Builder
	reportBuilder.WriteString("# ◆ orchkit GitHub Triage Report\n\n")
	reportBuilder.WriteString(fmt.Sprintf("**Repository:** %s/%s\n", owner, repo))
	reportBuilder.WriteString(fmt.Sprintf("**Generated:** %s\n", time.Now().Format("2006-01-02 15:04:05")))
	reportBuilder.WriteString(fmt.Sprintf("**Issues Triaged:** %d\n\n", len(triages)))
	reportBuilder.WriteString("---\n\n## Summary\n\n")
	reportBuilder.WriteString("| Issue | Title | Priority | Category | Action |\n")
	reportBuilder.WriteString("|-------|-------|----------|----------|--------|\n")
	for _, t := range triages {
		reportBuilder.WriteString(fmt.Sprintf("| #%d | %s | %s | %s | %s |\n",
			t.Number, truncate(t.Title, 40), t.Priority, t.Category, truncate(t.Action, 50)))
	}

	counts := map[string]int{}
	for _, t := range triages {
		counts[t.Priority]++
	}
	reportBuilder.WriteString("\n## Priority Breakdown\n\n")
	for _, p := range []string{"critical", "high", "medium", "low"} {
		if n, ok := counts[p]; ok {
			reportBuilder.WriteString(fmt.Sprintf("- **%s:** %s (%d)\n",
				p, strings.Repeat("█", n), n))
		}
	}

	reportBuilder.WriteString("\n## Detailed Triage\n\n")
	for _, t := range triages {
		reportBuilder.WriteString(fmt.Sprintf("### Issue #%d: %s\n\n", t.Number, t.Title))
		reportBuilder.WriteString(fmt.Sprintf("- **Priority:** %s\n", t.Priority))
		reportBuilder.WriteString(fmt.Sprintf("- **Category:** %s\n", t.Category))
		reportBuilder.WriteString(fmt.Sprintf("- **Action:** %s\n", t.Action))
		reportBuilder.WriteString(fmt.Sprintf("- **Comment:** %s\n\n", t.Comment))
		reportBuilder.WriteString(fmt.Sprintf("[View Issue](https://github.com/%s/%s/issues/%d)\n\n",
			owner, repo, t.Number))
		reportBuilder.WriteString("---\n\n")
	}
	reportBuilder.WriteString("*Generated by orchkit GitHub Automation Agent*\n")

	reportPath := "/tmp/orchkit-triage-report.md"
	writeStore := orchkit.NewMemStore()
	writeStore.Put(ctx, "content", reportBuilder.String())
	writeFlow := orchkit.NewFlow().Step("write_report", nodes.NewFSWrite(reportPath))
	_, err = orchkit.Run(ctx, writeFlow, writeStore, orchkit.RunOptions{Hooks: hooks})
	if err != nil {
		fmt.Printf("  write failed: %v\n", err)
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("◆ AGENT COMPLETE")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Issues triaged : %d\n", len(triages))
	fmt.Printf("Report         : %s\n\n", reportPath)
	fmt.Println(reportBuilder.String())
}

func parseTriage(number int, title, text string) Triage {
	t := Triage{Number: number, Title: title,
		Priority: "medium", Category: "feature", Action: "Review and assign"}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "PRIORITY:"):
			t.Priority = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "PRIORITY:")))
		case strings.HasPrefix(line, "CATEGORY:"):
			t.Category = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "CATEGORY:")))
		case strings.HasPrefix(line, "ACTION:"):
			t.Action = strings.TrimSpace(strings.TrimPrefix(line, "ACTION:"))
		case strings.HasPrefix(line, "COMMENT:"):
			t.Comment = strings.TrimSpace(strings.TrimPrefix(line, "COMMENT:"))
		}
	}
	return t
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	}
	return 0
}
