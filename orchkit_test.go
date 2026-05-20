package orchkit_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"orchkit"
)

// ----------------------------------------------------------------------------
// fakeNode — a test double. No real I/O, just echo or error on demand.
// ----------------------------------------------------------------------------

type fakeNode struct {
	name string
	out  orchkit.Output
	err  error
	calls int
}

func (f *fakeNode) Name() string { return f.name }
func (f *fakeNode) Schema() orchkit.Schema { return orchkit.Schema{} }
func (f *fakeNode) Execute(_ context.Context, _ orchkit.Input) (orchkit.Output, error) {
	f.calls++
	return f.out, f.err
}

// ----------------------------------------------------------------------------
// MemStore tests
// ----------------------------------------------------------------------------

func TestMemStore_PutGet(t *testing.T) {
	ctx := context.Background()
	s := orchkit.NewMemStore()

	if err := s.Put(ctx, "key", "value"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	v, ok, err := s.Get(ctx, "key")
	if err != nil || !ok || v != "value" {
		t.Fatalf("Get: got %v, ok=%v, err=%v", v, ok, err)
	}
}

func TestMemStore_MissingKey(t *testing.T) {
	ctx := context.Background()
	s := orchkit.NewMemStore()
	_, ok, err := s.Get(ctx, "missing")
	if err != nil || ok {
		t.Fatalf("expected miss, got ok=%v err=%v", ok, err)
	}
}

func TestMemStore_Snapshot(t *testing.T) {
	ctx := context.Background()
	s := orchkit.NewMemStore()
	_ = s.Put(ctx, "a", 1)
	_ = s.Put(ctx, "b", 2)

	snap, err := s.Snapshot(ctx)
	if err != nil || len(snap) != 2 {
		t.Fatalf("Snapshot: got %v err=%v", snap, err)
	}
}

// ----------------------------------------------------------------------------
// Run tests
// ----------------------------------------------------------------------------

func TestRun_SingleStep(t *testing.T) {
	ctx := context.Background()
	node := &fakeNode{name: "echo", out: orchkit.Output{"result": "ok"}}
	flow := orchkit.NewFlow().Step("step1", node)

	state, err := orchkit.Run(ctx, flow, orchkit.NewMemStore())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if node.calls != 1 {
		t.Fatalf("expected 1 call, got %d", node.calls)
	}
	if state["step1"] == nil {
		t.Fatal("expected step1 in state")
	}
}

func TestRun_MultiStep_StateFlows(t *testing.T) {
	ctx := context.Background()
	a := &fakeNode{name: "a", out: orchkit.Output{"x": 42}}
	b := &fakeNode{name: "b", out: orchkit.Output{"y": 99}}

	flow := orchkit.NewFlow().Step("stepA", a).Step("stepB", b)
	state, err := orchkit.Run(ctx, flow, orchkit.NewMemStore())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if state["stepA"] == nil || state["stepB"] == nil {
		t.Fatalf("expected both steps in state: %v", state)
	}
}

func TestRun_StopsOnError(t *testing.T) {
	ctx := context.Background()
	bad := &fakeNode{name: "bad", err: errors.New("boom")}
	never := &fakeNode{name: "never"}

	flow := orchkit.NewFlow().Step("bad", bad).Step("never", never)
	_, err := orchkit.Run(ctx, flow, orchkit.NewMemStore())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if never.calls != 0 {
		t.Fatal("second step should not have run")
	}
}

func TestRun_NilFlow(t *testing.T) {
	_, err := orchkit.Run(context.Background(), nil, orchkit.NewMemStore())
	if err == nil {
		t.Fatal("expected error for nil flow")
	}
}

func TestRun_NilStore_DefaultsToMem(t *testing.T) {
	node := &fakeNode{name: "n", out: orchkit.Output{"v": 1}}
	flow := orchkit.NewFlow().Step("s", node)
	_, err := orchkit.Run(context.Background(), flow, nil)
	if err != nil {
		t.Fatalf("expected nil store to use MemStore, got: %v", err)
	}
}

// ----------------------------------------------------------------------------
// Retry tests
// ----------------------------------------------------------------------------

func TestRetry_SucceedsFirstTry(t *testing.T) {
	node := &fakeNode{name: "n", out: orchkit.Output{"ok": true}}
	wrapped := orchkit.Retry(node, 3, 0)
	out, err := wrapped.Execute(context.Background(), nil)
	if err != nil || out["ok"] != true || node.calls != 1 {
		t.Fatalf("expected 1 call success, got calls=%d err=%v", node.calls, err)
	}
}

func TestRetry_RetriesOnError(t *testing.T) {
	attempt := 0
	node := &fakeNode{name: "n"}
	// Fail twice, succeed third.
	node.err = errors.New("fail")

	flaky := &flakyNode{failFor: 2, inner: &fakeNode{name: "n", out: orchkit.Output{"ok": true}}}
	wrapped := orchkit.Retry(flaky, 3, 0)
	out, err := wrapped.Execute(context.Background(), nil)
	_ = attempt
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if out["ok"] != true {
		t.Fatalf("unexpected output: %v", out)
	}
}

func TestRetry_ExhaustsAttempts(t *testing.T) {
	node := &fakeNode{name: "n", err: errors.New("always fails")}
	wrapped := orchkit.Retry(node, 3, 0)
	_, err := wrapped.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if node.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", node.calls)
	}
}

func TestRetry_RespectsContext(t *testing.T) {
	node := &fakeNode{name: "n", err: errors.New("fail")}
	wrapped := orchkit.Retry(node, 5, 10*time.Second) // long backoff

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := wrapped.Execute(ctx, nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

// flakyNode fails for the first `failFor` calls, then succeeds.
type flakyNode struct {
	failFor int
	calls   int
	inner   orchkit.Node
}

func (f *flakyNode) Name() string           { return f.inner.Name() }
func (f *flakyNode) Schema() orchkit.Schema { return f.inner.Schema() }
func (f *flakyNode) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	f.calls++
	if f.calls <= f.failFor {
		return nil, fmt.Errorf("flaky: simulated failure %d", f.calls)
	}
	return f.inner.Execute(ctx, in)
}
