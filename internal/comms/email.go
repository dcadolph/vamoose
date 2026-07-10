package comms

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// EmailConfig holds the SMTP settings for an email notifier.
type EmailConfig struct {
	// Host is the SMTP server hostname.
	Host string
	// Port is the SMTP server port, defaulting to 587.
	Port string
	// Username authenticates to the server, empty for no authentication.
	Username string
	// Password pairs with Username.
	Password string
	// From is the sender address.
	From string
}

// EmailNotifier sends a message as a short email over SMTP. The channel passed to
// Notify is the recipient address.
type EmailNotifier struct {
	// addr is the SMTP server host and port.
	addr string
	// from is the sender address.
	from string
	// auth authenticates to the server, nil when no username is set.
	auth smtp.Auth
	// send delivers the message, defaulting to smtp.SendMail and overridable in tests.
	send func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// NewEmailNotifier returns an email notifier for the given SMTP settings.
func NewEmailNotifier(cfg EmailConfig) *EmailNotifier {
	port := cfg.Port
	if port == "" {
		port = "587"
	}
	var auth smtp.Auth
	if cfg.Username != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}
	return &EmailNotifier{
		addr: net.JoinHostPort(cfg.Host, port),
		from: cfg.From,
		auth: auth,
		send: smtp.SendMail,
	}
}

// Notify sends text to the recipient address as a plain-text email, taking the subject
// from the first line of the text. It returns an error when the send fails, so a
// workflow surfaces the failure rather than dropping the message.
func (e *EmailNotifier) Notify(_ context.Context, to, text string) error {
	if to == "" {
		return fmt.Errorf("email notify: empty recipient")
	}
	subject := firstLine(text)
	if subject == "" {
		subject = "Calendar update"
	}
	msg := buildEmail(e.from, to, subject, text)
	if err := e.send(e.addr, e.auth, e.from, []string{to}, msg); err != nil {
		return fmt.Errorf("email notify: %w", err)
	}
	return nil
}

// buildEmail formats a plain-text email with the standard headers.
func buildEmail(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// firstLine returns the text up to the first newline, so an email subject stays to one
// line even when the body has several.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
