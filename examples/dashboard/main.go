package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"orchkit"
	"orchkit/dashboard"
	"orchkit/nodes"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Flow 1: fetch and parse JSON
	fetchFlow := orchkit.NewFlow().
		Step("fetch", nodes.NewHTTPGet("https://httpbin.org/json")).
		Step("parse", nodes.NewJSONParse("slideshow"))

	// Flow 2: system info
	sysFlow := orchkit.NewFlow().
		Step("hostname", nodes.NewShell("hostname")).
		Step("uptime", nodes.NewShell("uptime")).
		Step("disk", nodes.NewShell("df -h /"))

	// Flow 3: write timestamp — map stdout -> content explicitly
	writeFlow := orchkit.NewFlow().
		Step("timestamp", nodes.NewShell("date")).
		StepWith(orchkit.Step{
			ID:   "write",
			Node: nodes.NewFSWrite("/tmp/orchkit-dashboard-test.txt"),
			In: map[string]string{
				"timestamp.stdout": "content",
			},
		})

	d := dashboard.New(":9090")
	d.Register("fetch-and-parse", fetchFlow, orchkit.NewMemStore())
	d.Register("system-info", sysFlow, orchkit.NewMemStore())
	d.Register("write-timestamp", writeFlow, orchkit.NewMemStore())

	fmt.Println("orchkit dashboard running at http://localhost:9090")
	fmt.Println("Press Ctrl+C to stop.")

	if err := d.Serve(ctx); err != nil {
		log.Fatalf("dashboard: %v", err)
	}
}
