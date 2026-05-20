package orchkit_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

func tempDB(t *testing.T) (*orchkit.BoltStore, func()) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	store, err := orchkit.NewBoltStore(path)
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	return store, func() { store.Close() }
}

func TestBoltStore_PutGet(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.Put(ctx, "key", "value"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	v, ok, err := store.Get(ctx, "key")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if v != "value" {
		t.Fatalf("expected 'value', got %v", v)
	}
}

func TestBoltStore_MissingKey(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	_, ok, err := store.Get(context.Background(), "missing")
	if err != nil || ok {
		t.Fatalf("expected miss, got ok=%v err=%v", ok, err)
	}
}

func TestBoltStore_Snapshot(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	ctx := context.Background()
	_ = store.Put(ctx, "a", 1)
	_ = store.Put(ctx, "b", 2)
	_ = store.Put(ctx, "c", 3)

	snap, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(snap) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(snap))
	}
}

func TestBoltStore_Overwrite(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	ctx := context.Background()
	_ = store.Put(ctx, "key", "first")
	_ = store.Put(ctx, "key", "second")

	v, _, _ := store.Get(ctx, "key")
	if v != "second" {
		t.Fatalf("expected 'second', got %v", v)
	}
}

func TestBoltStore_PersistsAcrossOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")

	// Write in first open.
	store1, err := orchkit.NewBoltStore(path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	ctx := context.Background()
	_ = store1.Put(ctx, "survived", "yes")
	store1.Close()

	// Read in second open — different process simulation.
	store2, err := orchkit.NewBoltStore(path)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	defer store2.Close()

	v, ok, err := store2.Get(ctx, "survived")
	if err != nil || !ok {
		t.Fatalf("expected key to survive restart: ok=%v err=%v", ok, err)
	}
	if v != "yes" {
		t.Fatalf("expected 'yes', got %v", v)
	}
}

func TestBoltStore_Clear(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	ctx := context.Background()
	_ = store.Put(ctx, "a", 1)
	_ = store.Put(ctx, "b", 2)

	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	snap, _ := store.Snapshot(ctx)
	if len(snap) != 0 {
		t.Fatalf("expected empty store after Clear, got %d keys", len(snap))
	}
}

func TestBoltStore_ComplexValue(t *testing.T) {
	store, cleanup := tempDB(t)
	defer cleanup()

	ctx := context.Background()
	complex := map[string]any{
		"nested": map[string]any{"deep": true},
		"list":   []any{1, 2, 3},
		"num":    float64(42),
	}
	if err := store.Put(ctx, "complex", complex); err != nil {
		t.Fatalf("Put: %v", err)
	}

	v, ok, err := store.Get(ctx, "complex")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", v)
	}
	nested, _ := m["nested"].(map[string]any)
	if nested["deep"] != true {
		t.Fatalf("nested value wrong: %v", nested)
	}
}

func TestBoltStore_RunWithBoltStore(t *testing.T) {
	// Full integration: Run a real flow against BoltStore.
	dir := t.TempDir()
	store, err := orchkit.NewBoltStore(filepath.Join(dir, "flow.db"))
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}
	defer store.Close()

	node := &struct {
		orchkit.Node
	}{}
	_ = node

	// Use fakeNode from orchkit_test — inline it here.
	fake := &boltFakeNode{name: "step", out: orchkit.Output{"done": true}}
	flow := orchkit.NewFlow().Step("s1", fake)

	state, err := orchkit.Run(context.Background(), flow, store)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if state["s1"] == nil {
		t.Fatal("expected s1 in state")
	}

	// Reopen store — state must still be there.
	store.Close()
	store2, _ := orchkit.NewBoltStore(filepath.Join(dir, "flow.db"))
	defer store2.Close()

	snap, _ := store2.Snapshot(context.Background())
	if snap["s1"] == nil {
		t.Fatal("state did not persist after store reopen")
	}
}

type boltFakeNode struct {
	name string
	out  orchkit.Output
}

func (b *boltFakeNode) Name() string           { return b.name }
func (b *boltFakeNode) Schema() orchkit.Schema { return orchkit.Schema{} }
func (b *boltFakeNode) Execute(_ context.Context, _ orchkit.Input) (orchkit.Output, error) {
	return b.out, nil
}
