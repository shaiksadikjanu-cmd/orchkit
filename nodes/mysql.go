package nodes

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"orchkit"
)

// MySQL executes queries against a MySQL or MariaDB database.
//
// Example:
//
//	nodes.NewMySQL("user:pass@tcp(localhost:3306)/dbname", "SELECT * FROM users")
type MySQL struct {
	DSN   string
	Query string
}

func NewMySQL(dsn, query string) *MySQL {
	return &MySQL{DSN: dsn, Query: query}
}

func (m *MySQL) Name() string { return "mysql" }

func (m *MySQL) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Executes SQL queries against MySQL or MariaDB. Returns rows for SELECT, affected count for writes.",
		Params: map[string]any{
			"query": map[string]any{"type": "string", "desc": "SQL query or statement."},
			"args":  map[string]any{"type": "array", "desc": "Query parameters for prepared statements."},
			"dsn":   map[string]any{"type": "string", "desc": "MySQL DSN. Falls back to constructor."},
		},
	}
}

func (m *MySQL) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	dsn := m.DSN
	if v, ok := in["dsn"].(string); ok && v != "" {
		dsn = v
	}
	if dsn == "" {
		return nil, fmt.Errorf("mysql: DSN is required")
	}

	query := m.Query
	if v, ok := in["query"].(string); ok && v != "" {
		query = v
	}
	if query == "" {
		return nil, fmt.Errorf("mysql: query is required")
	}

	var args []any
	if v, ok := in["args"].([]any); ok {
		args = v
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql: open: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("mysql: ping: %w", err)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		result, execErr := db.ExecContext(ctx, query, args...)
		if execErr != nil {
			return nil, fmt.Errorf("mysql: %w", execErr)
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
		return nil, fmt.Errorf("mysql: columns: %w", err)
	}

	var result []any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("mysql: scan: %w", err)
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
