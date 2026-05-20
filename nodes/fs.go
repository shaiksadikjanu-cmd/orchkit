package nodes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"orchkit"
)

// ----------------------------------------------------------------------------
// FSRead — reads a file and returns its contents as a string.
// ----------------------------------------------------------------------------

type FSRead struct {
	Path string // if empty, taken from input["path"] at runtime
}

func NewFSRead(path string) *FSRead {
	return &FSRead{Path: path}
}

func (f *FSRead) Name() string { return "fs_read" }

func (f *FSRead) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Reads a file from disk and returns its contents as a string.",
		Params: map[string]any{
			"path": map[string]any{
				"type": "string",
				"desc": "Absolute or relative path to the file to read.",
			},
		},
	}
}

func (f *FSRead) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	path := f.Path
	if v, ok := in["path"].(string); ok && v != "" {
		path = v
	}
	if path == "" {
		return nil, fmt.Errorf("fs_read: no path provided")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fs_read: %w", err)
	}

	return orchkit.Output{
		"content": string(data),
		"path":    path,
		"size":    len(data),
	}, nil
}

// ----------------------------------------------------------------------------
// FSWrite — writes a string to a file. Creates directories if needed.
// ----------------------------------------------------------------------------

type FSWrite struct {
	Path string // if empty, taken from input["path"] at runtime
	Perm os.FileMode
}

func NewFSWrite(path string) *FSWrite {
	return &FSWrite{Path: path, Perm: 0644}
}

func (f *FSWrite) Name() string { return "fs_write" }

func (f *FSWrite) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Writes a string to a file on disk. Creates parent directories if they do not exist.",
		Params: map[string]any{
			"path": map[string]any{
				"type": "string",
				"desc": "Absolute or relative path to write to.",
			},
			"content": map[string]any{
				"type": "string",
				"desc": "String content to write.",
			},
		},
	}
}

func (f *FSWrite) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	path := f.Path
	if v, ok := in["path"].(string); ok && v != "" {
		path = v
	}
	if path == "" {
		return nil, fmt.Errorf("fs_write: no path provided")
	}

	content, ok := in["content"].(string)
	if !ok {
		return nil, fmt.Errorf("fs_write: input \"content\" is required and must be a string")
	}

	// Create parent directories if they don't exist.
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("fs_write: creating dirs: %w", err)
	}

	perm := f.Perm
	if perm == 0 {
		perm = 0644
	}

	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		return nil, fmt.Errorf("fs_write: %w", err)
	}

	return orchkit.Output{
		"path":  path,
		"size":  len(content),
		"wrote": true,
	}, nil
}
