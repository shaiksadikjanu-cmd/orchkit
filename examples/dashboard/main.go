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

	// Flow 1: fetch and parse
	fetchFlow := orchkit.NewFlow().
		Step("fetch", nodes.NewHTTPGet("https://httpbin.org/json")).
		Step("parse", nodes.NewJSONParse("slideshow"))

	// Flow 2: system info
	sysFlow := orchkit.NewFlow().
		Step("hostname", nodes.NewShell("hostname")).
		Step("uptime", nodes.NewShell("uptime")).
		Step("disk", nodes.NewShell("df -h /"))

	// Flow 3: write a timestamped file
	writeFlow := orchkit.NewFlow().
		Step("timestamp", nodes.NewShell("date")).
		Step("write", nodes.NewFSWrite("/tmp/orchkit-dashboard-test.txt"))

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
