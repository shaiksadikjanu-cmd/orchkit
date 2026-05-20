package nodes

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"orchkit"
)

// Redis executes GET, SET, DEL, LPUSH, RPOP commands against a Redis server.
// Uses raw RESP protocol — zero external dependencies.
//
// Example:
//
//	nodes.NewRedis("localhost:6379", "")
type Redis struct {
	Addr     string
	Password string
}

func NewRedis(addr, password string) *Redis {
	if addr == "" {
		addr = "localhost:6379"
	}
	return &Redis{Addr: addr, Password: password}
}

func (r *Redis) Name() string { return "redis" }

func (r *Redis) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Executes Redis commands: GET, SET, DEL, LPUSH, RPOP, EXPIRE, EXISTS.",
		Params: map[string]any{
			"command": map[string]any{"type": "string", "desc": "Redis command: GET | SET | DEL | LPUSH | RPOP | EXPIRE | EXISTS"},
			"key":     map[string]any{"type": "string", "desc": "Redis key."},
			"value":   map[string]any{"type": "string", "desc": "Value (SET, LPUSH)."},
			"ttl":     map[string]any{"type": "integer", "desc": "TTL in seconds (EXPIRE, or SET with expiry)."},
		},
	}
}

func (r *Redis) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	command, _ := in["command"].(string)
	key, _ := in["key"].(string)
	value, _ := in["value"].(string)

	if command == "" {
		return nil, fmt.Errorf("redis: command is required")
	}
	if key == "" {
		return nil, fmt.Errorf("redis: key is required")
	}

	conn, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", r.Addr)
	if err != nil {
		return nil, fmt.Errorf("redis: connect: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	// Authenticate if password set.
	if r.Password != "" {
		if err := r.send(rw, "AUTH", r.Password); err != nil {
			return nil, fmt.Errorf("redis: auth: %w", err)
		}
		if _, err := r.read(rw); err != nil {
			return nil, fmt.Errorf("redis: auth response: %w", err)
		}
	}

	var args []string
	cmd := strings.ToUpper(command)
	switch cmd {
	case "GET", "DEL", "EXISTS", "RPOP":
		args = []string{cmd, key}
	case "SET":
		args = []string{cmd, key, value}
		if ttl, ok := in["ttl"].(float64); ok && ttl > 0 {
			args = append(args, "EX", fmt.Sprintf("%d", int(ttl)))
		}
	case "LPUSH":
		args = []string{cmd, key, value}
	case "EXPIRE":
		ttl, _ := in["ttl"].(float64)
		args = []string{cmd, key, fmt.Sprintf("%d", int(ttl))}
	default:
		return nil, fmt.Errorf("redis: unsupported command %q", command)
	}

	if err := r.send(rw, args...); err != nil {
		return nil, fmt.Errorf("redis: send: %w", err)
	}

	result, err := r.read(rw)
	if err != nil {
		return nil, fmt.Errorf("redis: read: %w", err)
	}

	return orchkit.Output{"result": result, "key": key, "command": cmd}, nil
}

func (r *Redis) send(rw *bufio.ReadWriter, args ...string) error {
	fmt.Fprintf(rw, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(rw, "$%d\r\n%s\r\n", len(a), a)
	}
	return rw.Flush()
}

func (r *Redis) read(rw *bufio.ReadWriter) (any, error) {
	line, err := rw.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	switch line[0] {
	case '+':
		return line[1:], nil
	case '-':
		return nil, fmt.Errorf(line[1:])
	case ':':
		var n int
		fmt.Sscanf(line[1:], "%d", &n)
		return n, nil
	case '$':
		var size int
		fmt.Sscanf(line[1:], "%d", &size)
		if size == -1 {
			return nil, nil
		}
		buf := make([]byte, size+2)
		if _, err := rw.Read(buf); err != nil {
			return nil, err
		}
		return string(buf[:size]), nil
	default:
		return line, nil
	}
}
