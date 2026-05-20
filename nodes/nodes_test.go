package nodes_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/shaiksadikjanu-cmd/orchkit/nodes"
)

func TestHTTPGet_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		fmt.Fprint(w, `{"hello":"world"}`)
	}))
	defer srv.Close()

	node := nodes.NewHTTPGet(srv.URL)
	out, err := node.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["status"] != 200 {
		t.Fatalf("expected status 200, got %v", out["status"])
	}
	if out["body"] != `{"hello":"world"}` {
		t.Fatalf("unexpected body: %v", out["body"])
	}
}

func TestHTTPGet_URLFromInput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	node := nodes.NewHTTPGet("")
	out, err := node.Execute(context.Background(), map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["body"] != "ok" {
		t.Fatalf("expected body ok, got %v", out["body"])
	}
}

func TestHTTPGet_NoURL(t *testing.T) {
	node := nodes.NewHTTPGet("")
	_, err := node.Execute(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error when no URL provided")
	}
}

func TestHTTPGet_Non200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	node := nodes.NewHTTPGet(srv.URL)
	out, err := node.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["status"] != 404 {
		t.Fatalf("expected 404, got %v", out["status"])
	}
}

func TestHTTPGet_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	node := nodes.NewHTTPGet(srv.URL)
	_, err := node.Execute(ctx, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestHTTPGet_Schema(t *testing.T) {
	node := nodes.NewHTTPGet("")
	s := node.Schema()
	if s.Description == "" {
		t.Fatal("schema description must not be empty")
	}
	if _, ok := s.Params["url"]; !ok {
		t.Fatal("schema must declare url param")
	}
}

func TestJSONParse_FullObject(t *testing.T) {
	node := nodes.NewJSONParse("")
	out, err := node.Execute(context.Background(), map[string]any{
		"body": `{"name":"github.com/shaiksadikjanu-cmd/orchkit","version":1}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := out["value"].(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", out["value"])
	}
	if m["name"] != "github.com/shaiksadikjanu-cmd/orchkit" {
		t.Fatalf("expected name=orchkit, got %v", m["name"])
	}
}

func TestJSONParse_ExtractField(t *testing.T) {
	node := nodes.NewJSONParse("title")
	out, err := node.Execute(context.Background(), map[string]any{
		"body": `{"title":"hello","other":"ignored"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["value"] != "hello" {
		t.Fatalf("expected hello, got %v", out["value"])
	}
}

func TestJSONParse_FieldFromInput(t *testing.T) {
	node := nodes.NewJSONParse("")
	out, err := node.Execute(context.Background(), map[string]any{
		"body":  `{"x":42}`,
		"field": "x",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["value"] != float64(42) {
		t.Fatalf("expected 42, got %v", out["value"])
	}
}

func TestJSONParse_FieldNotFound(t *testing.T) {
	node := nodes.NewJSONParse("missing")
	_, err := node.Execute(context.Background(), map[string]any{
		"body": `{"exists":"yes"}`,
	})
	if err == nil {
		t.Fatal("expected error when field not found")
	}
}

func TestJSONParse_InvalidJSON(t *testing.T) {
	node := nodes.NewJSONParse("")
	_, err := node.Execute(context.Background(), map[string]any{
		"body": `not json at all`,
	})
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestJSONParse_NoBody(t *testing.T) {
	node := nodes.NewJSONParse("")
	_, err := node.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error when no body provided")
	}
}

func TestJSONParse_BodyFromStepNamespace(t *testing.T) {
	node := nodes.NewJSONParse("")
	out, err := node.Execute(context.Background(), map[string]any{
		"fetch": map[string]any{
			"body":   `{"result":"found"}`,
			"status": 200,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := out["value"].(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", out["value"])
	}
	if m["result"] != "found" {
		t.Fatalf("expected result=found, got %v", m["result"])
	}
}

func TestFSWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	node := nodes.NewFSWrite("")
	out, err := node.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "hello orchkit",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["wrote"] != true {
		t.Fatal("expected wrote=true")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "hello orchkit" {
		t.Fatalf("unexpected file content: %q", data)
	}
}

func TestFSWrite_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "file.txt")

	node := nodes.NewFSWrite("")
	_, err := node.Execute(context.Background(), map[string]any{
		"path": path, "content": "nested",
	})
	if err != nil {
		t.Fatalf("expected parent dirs to be created: %v", err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("file was not created")
	}
}

func TestFSWrite_NoPath(t *testing.T) {
	node := nodes.NewFSWrite("")
	_, err := node.Execute(context.Background(), map[string]any{"content": "data"})
	if err == nil {
		t.Fatal("expected error when no path")
	}
}

func TestFSWrite_NoContent(t *testing.T) {
	dir := t.TempDir()
	node := nodes.NewFSWrite("")
	_, err := node.Execute(context.Background(), map[string]any{
		"path": filepath.Join(dir, "file.txt"),
	})
	if err == nil {
		t.Fatal("expected error when no content")
	}
}

func TestFSRead_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "read.txt")
	_ = os.WriteFile(path, []byte("read me"), 0644)

	node := nodes.NewFSRead("")
	out, err := node.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["content"] != "read me" {
		t.Fatalf("unexpected content: %v", out["content"])
	}
	if out["size"] != 7 {
		t.Fatalf("unexpected size: %v", out["size"])
	}
}

func TestFSRead_FileNotFound(t *testing.T) {
	node := nodes.NewFSRead("")
	_, err := node.Execute(context.Background(), map[string]any{
		"path": "/tmp/orchkit-nonexistent-xyz.txt",
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestFSRead_NoPath(t *testing.T) {
	node := nodes.NewFSRead("")
	_, err := node.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error when no path")
	}
}

func TestFSWrite_ThenFSRead_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.txt")
	content := `{"key":"value","num":42}`

	write := nodes.NewFSWrite("")
	_, err := write.Execute(context.Background(), map[string]any{
		"path": path, "content": content,
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}

	read := nodes.NewFSRead("")
	out, err := read.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out["content"] != content {
		t.Fatalf("round-trip mismatch: got %v", out["content"])
	}
}

func TestShell_SimpleCommand(t *testing.T) {
	node := nodes.NewShell("echo hello")
	out, err := node.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stdout, _ := out["stdout"].(string)
	if stdout != "hello\n" {
		t.Fatalf("expected hello, got %q", stdout)
	}
	if out["exit_code"] != 0 {
		t.Fatalf("expected exit_code 0, got %v", out["exit_code"])
	}
}

func TestShell_CommandFromInput(t *testing.T) {
	node := nodes.NewShell("")
	out, err := node.Execute(context.Background(), map[string]any{
		"command": "echo from_input",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stdout, _ := out["stdout"].(string)
	if stdout != "from_input\n" {
		t.Fatalf("expected from_input, got %q", stdout)
	}
}

func TestShell_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\nexit 2\n"), 0755)
	node := nodes.NewShell("sh " + script)
	out, err := node.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if out["exit_code"] != 2 {
		t.Fatalf("expected exit_code 2, got %v", out["exit_code"])
	}
}

func TestShell_NoCommand(t *testing.T) {
	node := nodes.NewShell("")
	_, err := node.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error when no command")
	}
}

func TestShell_StderrCaptured(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "err.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\necho error >&2\n"), 0755)
	node := nodes.NewShell("sh " + script)
	out, err := node.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stderr, _ := out["stderr"].(string)
	if stderr != "error\n" {
		t.Fatalf("expected error on stderr, got %q", stderr)
	}
}

func TestShell_WorkingDir(t *testing.T) {
	dir := t.TempDir()
	node := nodes.NewShell("")
	out, err := node.Execute(context.Background(), map[string]any{
		"command": "pwd",
		"dir":     dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stdout, _ := out["stdout"].(string)
	if len(stdout) == 0 {
		t.Fatal("expected non-empty stdout from pwd")
	}
}

func TestAllNodes_SchemaCompliance(t *testing.T) {
	checks := []struct {
		name   string
		desc   string
		params map[string]any
	}{
		{nodes.NewHTTPGet("").Name(), nodes.NewHTTPGet("").Schema().Description, nodes.NewHTTPGet("").Schema().Params},
		{nodes.NewJSONParse("").Name(), nodes.NewJSONParse("").Schema().Description, nodes.NewJSONParse("").Schema().Params},
		{nodes.NewFSRead("").Name(), nodes.NewFSRead("").Schema().Description, nodes.NewFSRead("").Schema().Params},
		{nodes.NewFSWrite("").Name(), nodes.NewFSWrite("").Schema().Description, nodes.NewFSWrite("").Schema().Params},
		{nodes.NewShell("").Name(), nodes.NewShell("").Schema().Description, nodes.NewShell("").Schema().Params},
		{nodes.NewGroqLLM("", "").Name(), nodes.NewGroqLLM("", "").Schema().Description, nodes.NewGroqLLM("", "").Schema().Params},
		{nodes.NewGeminiLLM("", "").Name(), nodes.NewGeminiLLM("", "").Schema().Description, nodes.NewGeminiLLM("", "").Schema().Params},
	}
	for _, c := range checks {
		if c.name == "" {
			t.Errorf("a node has empty Name()")
		}
		if c.desc == "" {
			t.Errorf("node %q has empty Schema().Description", c.name)
		}
		if len(c.params) == 0 {
			t.Errorf("node %q has no Schema().Params", c.name)
		}
	}
}

func TestFlow_WriteRead_Integration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flow-output.json")

	payload, _ := json.Marshal(map[string]any{"built": "github.com/shaiksadikjanu-cmd/orchkit", "works": true})

	write := nodes.NewFSWrite(path)
	out, err := write.Execute(context.Background(), map[string]any{
		"content": string(payload),
	})
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if out["wrote"] != true {
		t.Fatal("wrote must be true")
	}

	read := nodes.NewFSRead(path)
	out2, err := read.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(out2["content"].(string)), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if result["built"] != "github.com/shaiksadikjanu-cmd/orchkit" {
		t.Fatalf("expected built=orchkit, got %v", result["built"])
	}
}
