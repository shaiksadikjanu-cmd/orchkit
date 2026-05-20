package nodes

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strings"

	"orchkit"
)

// CSVRead reads a CSV file and returns rows as a slice of maps.
// First row is treated as headers. Each subsequent row becomes a map
// of header->value.
//
// Example:
//
//	nodes.NewCSVRead("/data/users.csv")
type CSVRead struct {
	Path      string
	Delimiter rune // defaults to ','
}

func NewCSVRead(path string) *CSVRead {
	return &CSVRead{Path: path, Delimiter: ','}
}

func (c *CSVRead) Name() string { return "csv_read" }

func (c *CSVRead) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Reads a CSV file and returns rows as a list of objects keyed by header names.",
		Params: map[string]any{
			"path":      map[string]any{"type": "string", "desc": "Path to the CSV file."},
			"delimiter": map[string]any{"type": "string", "desc": "Field delimiter. Defaults to comma."},
		},
	}
}

func (c *CSVRead) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	path := c.Path
	if v, ok := in["path"].(string); ok && v != "" {
		path = v
	}
	if path == "" {
		return nil, fmt.Errorf("csv_read: no path provided")
	}

	delim := c.Delimiter
	if v, ok := in["delimiter"].(string); ok && v != "" {
		delim = rune(v[0])
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("csv_read: open: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = delim
	r.TrimLeadingSpace = true

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv_read: parse: %w", err)
	}
	if len(records) == 0 {
		return orchkit.Output{"rows": []any{}, "count": 0}, nil
	}

	headers := records[0]
	rows := make([]any, 0, len(records)-1)
	for _, record := range records[1:] {
		row := map[string]any{}
		for i, h := range headers {
			if i < len(record) {
				row[strings.TrimSpace(h)] = record[i]
			}
		}
		rows = append(rows, row)
	}

	return orchkit.Output{
		"rows":    rows,
		"count":   len(rows),
		"headers": headers,
	}, nil
}
