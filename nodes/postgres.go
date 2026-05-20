package nodes

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
	"github.com/shaiksadikjanu-cmd/orchkit"
)

// Postgres executes queries against a PostgreSQL database.
// Uses the same lib/pq driver already in go.mod from pgstore.
//
// Example:
//
//	nodes.NewPostgres("postgres://user:pass@localhost/db?sslmode=disable")
type Postgres struct {
	DSN   string
	Query string
}

func NewPostgres(dsn, query string) *Postgres {
	return &Postgres{DSN: dsn, Query: query}
}

func (p *Postgres) Name() string { return "postgres" }

func (p *Postgres) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Executes SQL queries against PostgreSQL. Returns rows for SELECT, affected count for writes.",
		Params: map[string]any{
			"query": map[string]any{"type": "string", "desc": "SQL query or statement."},
			"args":  map[string]any{"type": "array", "desc": "Query parameters for prepared statements."},
			"dsn":   map[string]any{"type": "string", "desc": "Postgres connection string. Falls back to constructor."},
		},
	}
}

func (p *Postgres) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	dsn := p.DSN
	if v, ok := in["dsn"].(string); ok && v != "" {
		dsn = v
	}
	if dsn == "" {
		return nil, fmt.Errorf("postgres: no DSN provided")
	}

	query := p.Query
	if v, ok := in["query"].(string); ok && v != "" {
		query = v
	}
	if query == "" {
		return nil, fmt.Errorf("postgres: no query provided")
	}

	var args []any
	if v, ok := in["args"].([]any); ok {
		args = v
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		// Try as exec (INSERT/UPDATE/DELETE).
		result, execErr := db.ExecContext(ctx, query, args...)
		if execErr != nil {
			return nil, fmt.Errorf("postgres: %w", execErr)
		}
		affected, _ := result.RowsAffected()
		return orchkit.Output{"rows_affected": affected}, nil
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("postgres: columns: %w", err)
	}

	var result []any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("postgres: scan: %w", err)
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
	return orchkit.Output{"rows": result, "count": len(result)}, nil
}
