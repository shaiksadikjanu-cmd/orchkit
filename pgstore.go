package orchkit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/lib/pq"
)

// PGStore is a persistent Store backed by PostgreSQL.
// Use this when you need distributed access or multiple processes
// reading/writing the same flow state.
//
// Schema (run once before use):
//
//	CREATE TABLE IF NOT EXISTS orchkit_store (
//	    key  TEXT PRIMARY KEY,
//	    val  JSONB NOT NULL,
//	    updated_at TIMESTAMPTZ DEFAULT now()
//	);
//
// Usage:
//
//	store, err := orchkit.NewPGStore("postgres://user:pass@localhost/db?sslmode=disable")
//	defer store.Close()
type PGStore struct {
	db    *sql.DB
	table string
}

func NewPGStore(dsn string) (*PGStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("pgstore: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pgstore: ping: %w", err)
	}
	s := &PGStore{db: db, table: "orchkit_store"}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("pgstore: migrate: %w", err)
	}
	return s, nil
}

func (p *PGStore) migrate() error {
	_, err := p.db.Exec(fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			key        TEXT PRIMARY KEY,
			val        JSONB NOT NULL,
			updated_at TIMESTAMPTZ DEFAULT now()
		)`, p.table))
	return err
}

func (p *PGStore) Close() error { return p.db.Close() }

func (p *PGStore) Get(ctx context.Context, key string) (any, bool, error) {
	var raw []byte
	err := p.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT val FROM %s WHERE key = $1`, p.table), key,
	).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("pgstore: get %q: %w", key, err)
	}
	var val any
	if err := json.Unmarshal(raw, &val); err != nil {
		return nil, false, fmt.Errorf("pgstore: unmarshal %q: %w", key, err)
	}
	return val, true, nil
}

func (p *PGStore) Put(ctx context.Context, key string, val any) error {
	raw, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("pgstore: marshal %q: %w", key, err)
	}
	_, err = p.db.ExecContext(ctx, fmt.Sprintf(`
		INSERT INTO %s (key, val, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE
		SET val = EXCLUDED.val, updated_at = now()
	`, p.table), key, raw)
	if err != nil {
		return fmt.Errorf("pgstore: put %q: %w", key, err)
	}
	return nil
}

func (p *PGStore) Snapshot(ctx context.Context) (map[string]any, error) {
	rows, err := p.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT key, val FROM %s`, p.table))
	if err != nil {
		return nil, fmt.Errorf("pgstore: snapshot: %w", err)
	}
	defer rows.Close()

	out := map[string]any{}
	for rows.Next() {
		var key string
		var raw []byte
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, fmt.Errorf("pgstore: scan: %w", err)
		}
		var val any
		if err := json.Unmarshal(raw, &val); err != nil {
			return nil, fmt.Errorf("pgstore: unmarshal: %w", err)
		}
		out[key] = val
	}
	return out, rows.Err()
}

func (p *PGStore) Clear(ctx context.Context) error {
	_, err := p.db.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM %s`, p.table))
	return err
}
