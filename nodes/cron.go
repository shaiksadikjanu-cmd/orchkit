package nodes

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Cron waits until the next occurrence of a cron schedule, then fires.
// Supports standard 5-field cron: minute hour day month weekday.
// Special: @hourly @daily @weekly @monthly
//
// Example:
//
//	nodes.NewCron("0 9 * * 1-5")   // 9am weekdays
//	nodes.NewCron("@daily")         // midnight every day
type Cron struct {
	Schedule string
}

func NewCron(schedule string) *Cron {
	return &Cron{Schedule: schedule}
}

func (c *Cron) Name() string { return "cron" }

func (c *Cron) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Waits until the next cron schedule occurrence, then fires. Use in a LoopNode for recurring triggers.",
		Params: map[string]any{
			"schedule": map[string]any{"type": "string", "desc": "Cron expression (5 fields) or @daily/@hourly/@weekly/@monthly."},
		},
	}
}

func (c *Cron) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	schedule := c.Schedule
	if v, ok := in["schedule"].(string); ok && v != "" {
		schedule = v
	}
	if schedule == "" {
		return nil, fmt.Errorf("cron: schedule is required")
	}

	// Expand aliases.
	switch schedule {
	case "@hourly":
		schedule = "0 * * * *"
	case "@daily", "@midnight":
		schedule = "0 0 * * *"
	case "@weekly":
		schedule = "0 0 * * 0"
	case "@monthly":
		schedule = "0 0 1 * *"
	}

	next, err := nextCron(schedule, time.Now())
	if err != nil {
		return nil, fmt.Errorf("cron: %w", err)
	}

	wait := time.Until(next)
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("cron: context cancelled while waiting: %w", ctx.Err())
	case <-time.After(wait):
	}

	return orchkit.Output{
		"fired_at": time.Now().Format(time.RFC3339),
		"schedule": schedule,
	}, nil
}

// nextCron computes the next time a cron expression fires after `from`.
func nextCron(expr string, from time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	t := from.Add(time.Minute).Truncate(time.Minute)
	// Search up to 1 year ahead.
	limit := from.Add(366 * 24 * time.Hour)

	for t.Before(limit) {
		if !matchField(fields[1], t.Hour(), 0, 23) {
			t = t.Add(time.Hour).Truncate(time.Hour)
			continue
		}
		if !matchField(fields[0], t.Minute(), 0, 59) {
			t = t.Add(time.Minute)
			continue
		}
		if !matchField(fields[2], t.Day(), 1, 31) {
			t = t.AddDate(0, 0, 1)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}
		if !matchField(fields[3], int(t.Month()), 1, 12) {
			t = t.AddDate(0, 1, 0)
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
			continue
		}
		if !matchField(fields[4], int(t.Weekday()), 0, 6) {
			t = t.AddDate(0, 0, 1)
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("no match found within 1 year")
}

func matchField(field string, val, min, max int) bool {
	if field == "*" {
		return true
	}
	// Ranges: 1-5
	if strings.Contains(field, "-") {
		parts := strings.SplitN(field, "-", 2)
		lo, _ := strconv.Atoi(parts[0])
		hi, _ := strconv.Atoi(parts[1])
		return val >= lo && val <= hi
	}
	// Lists: 1,2,3
	for _, p := range strings.Split(field, ",") {
		n, _ := strconv.Atoi(strings.TrimSpace(p))
		if n == val {
			return true
		}
	}
	// Step: */5
	if strings.HasPrefix(field, "*/") {
		step, _ := strconv.Atoi(field[2:])
		if step > 0 {
			return (val-min)%step == 0
		}
	}
	n, _ := strconv.Atoi(field)
	return n == val
}
