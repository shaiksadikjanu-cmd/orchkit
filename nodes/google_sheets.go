package nodes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"orchkit"
)

// GoogleSheets reads and writes Google Sheets via the Sheets API v4.
// Requires an OAuth2 access token or service account token.
// Actions: read, write, append, clear.
//
// Example:
//
//	nodes.NewGoogleSheets("ya29.access_token", "spreadsheet_id")
type GoogleSheets struct {
	Token         string
	SpreadsheetID string
	client        *http.Client
}

func NewGoogleSheets(token, spreadsheetID string) *GoogleSheets {
	return &GoogleSheets{
		Token:         token,
		SpreadsheetID: spreadsheetID,
		client:        &http.Client{Timeout: 30 * time.Second},
	}
}

func (g *GoogleSheets) Name() string { return "google_sheets" }

func (g *GoogleSheets) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Reads and writes Google Sheets. Actions: read, write, append, clear.",
		Params: map[string]any{
			"action":         map[string]any{"type": "string", "desc": "read | write | append | clear"},
			"range":          map[string]any{"type": "string", "desc": "A1 notation e.g. Sheet1!A1:D10"},
			"values":         map[string]any{"type": "array", "desc": "2D array of values to write/append."},
			"spreadsheet_id": map[string]any{"type": "string", "desc": "Spreadsheet ID. Falls back to constructor."},
		},
	}
}

func (g *GoogleSheets) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	spreadsheetID := g.SpreadsheetID
	if v, ok := in["spreadsheet_id"].(string); ok && v != "" {
		spreadsheetID = v
	}
	if spreadsheetID == "" {
		return nil, fmt.Errorf("google_sheets: spreadsheet_id required")
	}

	cellRange, _ := in["range"].(string)
	if cellRange == "" {
		cellRange = "Sheet1"
	}

	action, _ := in["action"].(string)
	if action == "" {
		action = "read"
	}

	base := fmt.Sprintf("https://sheets.googleapis.com/v4/spreadsheets/%s", spreadsheetID)

	switch action {
	case "read":
		url := fmt.Sprintf("%s/values/%s", base, cellRange)
		return g.get(ctx, url)

	case "write":
		values, _ := in["values"].([]any)
		if values == nil {
			return nil, fmt.Errorf("google_sheets: values required for write")
		}
		url := fmt.Sprintf("%s/values/%s?valueInputOption=USER_ENTERED", base, cellRange)
		body := map[string]any{"range": cellRange, "majorDimension": "ROWS", "values": values}
		return g.put(ctx, url, body)

	case "append":
		values, _ := in["values"].([]any)
		if values == nil {
			return nil, fmt.Errorf("google_sheets: values required for append")
		}
		url := fmt.Sprintf("%s/values/%s:append?valueInputOption=USER_ENTERED", base, cellRange)
		body := map[string]any{"range": cellRange, "majorDimension": "ROWS", "values": values}
		return g.post(ctx, url, body)

	case "clear":
		url := fmt.Sprintf("%s/values/%s:clear", base, cellRange)
		return g.post(ctx, url, map[string]any{})

	default:
		return nil, fmt.Errorf("google_sheets: unknown action %q", action)
	}
}

func (g *GoogleSheets) get(ctx context.Context, url string) (orchkit.Output, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return g.do(req)
}

func (g *GoogleSheets) post(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return g.do(req)
}

func (g *GoogleSheets) put(ctx context.Context, url string, body map[string]any) (orchkit.Output, error) {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	return g.do(req)
}

func (g *GoogleSheets) do(req *http.Request) (orchkit.Output, error) {
	req.Header.Set("authorization", "Bearer "+g.Token)
	req.Header.Set("content-type", "application/json")
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google_sheets: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("google_sheets: api error %d: %s", resp.StatusCode, body)
	}
	var result any
	json.Unmarshal(body, &result)
	return orchkit.Output{"result": result, "status": resp.StatusCode}, nil
}

// googleSheetsHTTPClient is exported for test injection
var _ = strings.ToLower // keep import
