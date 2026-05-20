package nodes

import (
	"context"
	"database/sql"
	"fmt"

	"orchkit"
	_ "modernc.org/sqlite"
)

// SQLite executes a query or statement against an embedded SQLite database.
// No server needed — single file on disk.
//
// For SELECT: returns rows as []map[string]any under "rows" key.
// For INSERT/UPDATE/DELETE: returns rows_affected and last_insert_id.
//
// Example:
//
//	nodes.NewSQLite("/data/app.db", "SELECT * FROM users WHERE active = ?", true)
type SQLite struct {
	DSN   string // path to .db file, e.g. "/data/app.db"
	Query string // if empty, taken from input["query"] at runtime
}

func NewSQLite(dsn, query string) *SQLite {
	return &SQLite{DSN: dsn, Query: query}
}

func (s *SQLite) Name() string { return "sqlite" }

func (s *SQLite) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Executes a SQL query against an embedded SQLite database. Returns rows for SELECT, affected count for writes.",
		Params: map[string]any{
			"query":  map[string]any{"type": "string", "desc": "SQL query or statement."},
			"args":   map[string]any{"type": "array", "desc": "Optional query parameters (for prepared statements)."},
			"dsn":    map[string]any{"type": "string", "desc": "Path to SQLite file. Falls back to constructor DSN."},
		},
	}
}

func (s *SQLite) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	dsn := s.DSN
	if v, ok := in["dsn"].(string); ok && v != "" {
		dsn = v
	}
	if dsn == "" {
		return nil, fmt.Errorf("sqlite: no DSN provided")
	}

	query := s.Query
	if v, ok := in["query"].(string); ok && v != "" {
		query = v
	}
	if query == "" {
		return nil, fmt.Errorf("sqlite: no query provided")
	}

	// Build args slice.
	var args []any
	if v, ok := in["args"].([]any); ok {
		args = v
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	defer db.Close()

	// Try as a query first (SELECT).
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		// Not a SELECT — try as Exec (INSERT/UPDATE/DELETE/CREATE).
		result, execErr := db.ExecContext(ctx, query, args...)
		if execErr != nil {
			return nil, fmt.Errorf("sqlite: %w", execErr)
		}
		affected, _ := result.RowsAffected()
		lastID, _ := result.LastInsertId()
		return orchkit.Output{
			"rows_affected":  affected,
			"last_insert_id": lastID,
		}, nil
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sqlite: columns: %w", err)
	}

	var result []any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("sqlite: scan: %w", err)
		}
		row := map[string]any{}
		for i, col := range cols {
			row[col] = vals[i]
		}
		result = append(result, row)
	}
	if result == nil {
		result = []any{}
	}

	return orchkit.Output{
		"rows":  result,
		"count": len(result),
	}, nil
}
