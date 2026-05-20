package nodes

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"github.com/shaiksadikjanu-cmd/orchkit"
)

// SSH runs commands on remote servers over SSH.
// Supports password and private key authentication.
//
// Example (password):
//
//	nodes.NewSSH("user@host:22", "password", "")
//
// Example (private key):
//
//	nodes.NewSSH("user@host:22", "", "/home/user/.ssh/id_rsa")
type SSH struct {
	Address    string // user@host:port
	Password   string
	PrivateKey string // path to private key file
	Command    string // optional default command
}

func NewSSH(address, password, privateKey string) *SSH {
	return &SSH{Address: address, Password: password, PrivateKey: privateKey}
}

func (s *SSH) Name() string { return "ssh" }

func (s *SSH) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Runs a command on a remote server via SSH. Supports password and private key auth.",
		Params: map[string]any{
			"command":     map[string]any{"type": "string", "desc": "Shell command to run remotely."},
			"address":     map[string]any{"type": "string", "desc": "user@host:port. Falls back to constructor."},
			"timeout_sec": map[string]any{"type": "integer", "desc": "Connection timeout in seconds. Default 30."},
		},
	}
}

func (s *SSH) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	command := s.Command
	if v, ok := in["command"].(string); ok && v != "" {
		command = v
	}
	if command == "" {
		return nil, fmt.Errorf("ssh: command is required")
	}

	address := s.Address
	if v, ok := in["address"].(string); ok && v != "" {
		address = v
	}

	timeoutSec := 30
	if v, ok := in["timeout_sec"].(float64); ok && v > 0 {
		timeoutSec = int(v)
	}

	// Parse user@host:port.
	user, hostport, err := parseSSHAddress(address)
	if err != nil {
		return nil, fmt.Errorf("ssh: %w", err)
	}

	// Build auth methods.
	var authMethods []ssh.AuthMethod
	if s.Password != "" {
		authMethods = append(authMethods, ssh.Password(s.Password))
	}
	if s.PrivateKey != "" {
		key, err := os.ReadFile(s.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("ssh: read private key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("ssh: parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("ssh: provide password or private key path")
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // production: use known_hosts
		Timeout:         time.Duration(timeoutSec) * time.Second,
	}

	dialer := &net.Dialer{}
	netConn, err := dialer.DialContext(ctx, "tcp", hostport)
	if err != nil {
		return nil, fmt.Errorf("ssh: dial: %w", err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, hostport, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh: handshake: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh: session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	exitCode := 0
	if err := session.Run(command); err != nil {
		if exitErr, ok := err.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			return nil, fmt.Errorf("ssh: run: %w", err)
		}
	}

	return orchkit.Output{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": exitCode,
		"address":   address,
	}, nil
}

func parseSSHAddress(addr string) (user, hostport string, err error) {
	// Expected: user@host:port or user@host (defaults port 22)
	atIdx := -1
	for i, c := range addr {
		if c == '@' {
			atIdx = i
		}
	}
	if atIdx < 0 {
		return "", "", fmt.Errorf("address must be user@host:port, got %q", addr)
	}
	user = addr[:atIdx]
	hostport = addr[atIdx+1:]
	if !hasPort(hostport) {
		hostport = hostport + ":22"
	}
	return user, hostport, nil
}

func hasPort(s string) bool {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return true
		}
		if s[i] == ']' {
			break
		}
	}
	return false
}
