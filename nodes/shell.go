package nodes

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"orchkit"
)

// Shell runs a shell command and returns stdout, stderr, and exit code.
// This node is intentionally simple: one command string, no shell injection
// protection beyond what the caller provides. Use with trusted input only.
//
// Example in a flow:
//
//	nodes.NewShell("ls -la /tmp")
//	nodes.NewShell("")   // command comes from input["command"] at runtime
type Shell struct {
	Command string // if empty, taken from input["command"] at runtime
	Dir     string // working directory; empty = inherit from parent process
}

func NewShell(command string) *Shell {
	return &Shell{Command: command}
}

func (s *Shell) Name() string { return "shell" }

func (s *Shell) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Runs a shell command and returns stdout, stderr, and exit code.",
		Params: map[string]any{
			"command": map[string]any{
				"type": "string",
				"desc": "Shell command to run. Falls back to constructor command if absent.",
			},
			"dir": map[string]any{
				"type": "string",
				"desc": "Working directory. Optional.",
			},
		},
	}
}

func (s *Shell) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	command := s.Command
	if v, ok := in["command"].(string); ok && v != "" {
		command = v
	}
	if command == "" {
		return nil, fmt.Errorf("shell: no command provided")
	}

	dir := s.Dir
	if v, ok := in["dir"].(string); ok && v != "" {
		dir = v
	}

	// Split on whitespace for simple commands.
	// For complex commands with pipes/redirects, wrap in sh -c.
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("shell: empty command after parsing")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			// Non-zero exit is not a Go error — caller inspects exit_code.
			err = nil
		} else {
			// Real error: command not found, permission denied, etc.
			return nil, fmt.Errorf("shell: %w", err)
		}
	}

	return orchkit.Output{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": exitCode,
	}, err
}
