package nodes

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"github.com/shaiksadikjanu-cmd/orchkit"
)

// SMTP sends an email via any SMTP server.
// Works with Gmail, SendGrid, Mailgun, local relay — anything SMTP.
//
// Example (Gmail):
//
//	nodes.NewSMTP("smtp.gmail.com", 587, "you@gmail.com", "app-password")
type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string // defaults to Username
}

func NewSMTP(host string, port int, username, password string) *SMTP {
	return &SMTP{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		From:     username,
	}
}

func (s *SMTP) Name() string { return "smtp" }

func (s *SMTP) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Sends an email via SMTP. Works with Gmail, SendGrid, Mailgun, or any SMTP server.",
		Params: map[string]any{
			"to":      map[string]any{"type": "string", "desc": "Recipient email address. Multiple: comma-separated."},
			"subject": map[string]any{"type": "string", "desc": "Email subject line."},
			"body":    map[string]any{"type": "string", "desc": "Email body (plain text)."},
			"from":    map[string]any{"type": "string", "desc": "Sender address. Defaults to SMTP username."},
		},
	}
}

func (s *SMTP) Execute(_ context.Context, in orchkit.Input) (orchkit.Output, error) {
	to, ok := in["to"].(string)
	if !ok || to == "" {
		return nil, fmt.Errorf("smtp: 'to' is required")
	}
	subject, _ := in["subject"].(string)
	body, _ := in["body"].(string)
	from := s.From
	if v, ok := in["from"].(string); ok && v != "" {
		from = v
	}

	recipients := strings.Split(to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	msg := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		from, to, subject, body,
	))

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)

	// Use STARTTLS for port 587, plain TLS for 465, none for 25.
	var err error
	switch s.Port {
	case 465:
		tlsCfg := &tls.Config{ServerName: s.Host}
		conn, dialErr := tls.Dial("tcp", addr, tlsCfg)
		if dialErr != nil {
			return nil, fmt.Errorf("smtp: tls dial: %w", dialErr)
		}
		defer conn.Close()
		client, clientErr := smtp.NewClient(conn, s.Host)
		if clientErr != nil {
			return nil, fmt.Errorf("smtp: client: %w", clientErr)
		}
		if err = client.Auth(auth); err != nil {
			return nil, fmt.Errorf("smtp: auth: %w", err)
		}
		if err = client.Mail(from); err != nil {
			return nil, fmt.Errorf("smtp: mail from: %w", err)
		}
		for _, r := range recipients {
			if err = client.Rcpt(r); err != nil {
				return nil, fmt.Errorf("smtp: rcpt %s: %w", r, err)
			}
		}
		w, err := client.Data()
		if err != nil {
			return nil, fmt.Errorf("smtp: data: %w", err)
		}
		if _, err = w.Write(msg); err != nil {
			return nil, fmt.Errorf("smtp: write: %w", err)
		}
		w.Close()
		client.Quit()
	default:
		// STARTTLS (587) or plain (25)
		tlsCfg := &tls.Config{ServerName: s.Host}
		conn, dialErr := net.Dial("tcp", addr)
		if dialErr != nil {
			return nil, fmt.Errorf("smtp: dial: %w", dialErr)
		}
		client, clientErr := smtp.NewClient(conn, s.Host)
		if clientErr != nil {
			return nil, fmt.Errorf("smtp: client: %w", clientErr)
		}
		defer client.Quit()
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err = client.StartTLS(tlsCfg); err != nil {
				return nil, fmt.Errorf("smtp: starttls: %w", err)
			}
		}
		if err = client.Auth(auth); err != nil {
			return nil, fmt.Errorf("smtp: auth: %w", err)
		}
		if err = client.Mail(from); err != nil {
			return nil, fmt.Errorf("smtp: mail from: %w", err)
		}
		for _, r := range recipients {
			if err = client.Rcpt(r); err != nil {
				return nil, fmt.Errorf("smtp: rcpt %s: %w", r, err)
			}
		}
		w, err := client.Data()
		if err != nil {
			return nil, fmt.Errorf("smtp: data: %w", err)
		}
		if _, err = w.Write(msg); err != nil {
			return nil, fmt.Errorf("smtp: write: %w", err)
		}
		w.Close()
	}

	return orchkit.Output{
		"sent":       true,
		"recipients": recipients,
		"subject":    subject,
	}, nil
}
