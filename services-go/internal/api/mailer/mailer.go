// Package mailer sends the few emails this application has to send.
//
// There are only three -- a password reset, an invitation, and a confirmation
// -- and none of them is worth a template engine or a queue. What matters
// instead is what happens when there is no mail server, which on a
// self-hosted instance is the usual case: sending has to fail in a way the
// caller can tell apart from "sent", so that a password reset can say "ask
// your administrator" rather than claiming to have sent something.
package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// ErrNotConfigured is returned when no mail server has been set up.
//
// A distinct error rather than a silent success: every caller has something
// better to do than pretend, and the difference is the whole reason this is
// worth checking.
var ErrNotConfigured = errors.New("no mail server is configured")

// Settings is what this package needs, as the site settings hold it.
type Settings interface {
	SMTP() Config
	SiteURL() string
	AppName() string
}

// Config is a mail server, as configured.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	// Secure means TLS from the first byte, rather than STARTTLS after
	// connecting. Port 465 wants this; 587 wants STARTTLS.
	Secure bool
	// IgnoreTLS skips certificate checking, which a self-signed relay on a
	// private network needs and nothing else should.
	IgnoreTLS bool
	From      string
	ReplyTo   string
}

func (c Config) configured() bool {
	return strings.TrimSpace(c.Host) != "" && strings.TrimSpace(c.From) != ""
}

// Message is one email.
type Message struct {
	To      string
	Subject string
	// Text is the whole body. No HTML: these messages are four lines and a
	// link, and an HTML part would only be a second thing to keep in step.
	Text string
}

// Mailer sends messages.
type Mailer struct {
	settings Settings
}

// New builds one.
func New(settings Settings) *Mailer {
	return &Mailer{settings: settings}
}

// Configured says whether there is anywhere to send to.
func (m *Mailer) Configured() bool {
	return m.settings.SMTP().configured()
}

// Send delivers one message, or answers with why it could not.
func (m *Mailer) Send(ctx context.Context, message Message) error {
	config := m.settings.SMTP()
	if !config.configured() {
		return ErrNotConfigured
	}

	address := net.JoinHostPort(config.Host, fmt.Sprint(portOr(config.Port, 587)))
	body := m.compose(config, message)

	// A deadline on the whole exchange: an unreachable relay otherwise holds
	// the request open until the client gives up, and the person sees a page
	// that never answers rather than an error they can act on.
	deadline, stop := context.WithTimeout(ctx, 20*time.Second)
	defer stop()

	done := make(chan error, 1)
	go func() { done <- m.deliver(config, address, message.To, body) }()

	select {
	case err := <-done:
		return err
	case <-deadline.Done():
		return fmt.Errorf("the mail server did not answer in time")
	}
}

func (m *Mailer) deliver(config Config, address, to string, body []byte) error {
	var client *smtp.Client
	var err error

	if config.Secure {
		connection, dialErr := tls.Dial("tcp", address, &tls.Config{
			ServerName:         config.Host,
			InsecureSkipVerify: config.IgnoreTLS, //nolint:gosec // the operator asked for this
			MinVersion:         tls.VersionTLS12,
		})
		if dialErr != nil {
			return dialErr
		}
		client, err = smtp.NewClient(connection, config.Host)
	} else {
		client, err = smtp.Dial(address)
	}
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if !config.Secure {
		// STARTTLS when the server offers it. Not required, because a relay on
		// a private network commonly does not, and refusing to send at all
		// would be a worse answer than sending over a link the operator chose.
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{
				ServerName:         config.Host,
				InsecureSkipVerify: config.IgnoreTLS, //nolint:gosec // the operator asked for this
				MinVersion:         tls.VersionTLS12,
			}); err != nil {
				return err
			}
		}
	}

	if config.User != "" {
		if err := client.Auth(smtp.PlainAuth("", config.User, config.Password, config.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(config.From); err != nil {
		return err
	}
	if err := client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(body); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

// compose builds the message, headers and all.
func (m *Mailer) compose(config Config, message Message) []byte {
	var out strings.Builder
	out.WriteString("From: " + header(m.settings.AppName()) + " <" + config.From + ">\r\n")
	out.WriteString("To: " + message.To + "\r\n")
	if config.ReplyTo != "" {
		out.WriteString("Reply-To: " + config.ReplyTo + "\r\n")
	}
	// Encoded, so a subject with a name in it survives a non-ASCII character.
	out.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", message.Subject) + "\r\n")
	out.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
	out.WriteString("MIME-Version: 1.0\r\n")
	out.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	out.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	out.WriteString("\r\n")
	// Bare newlines are not valid in a message body.
	out.WriteString(strings.ReplaceAll(
		strings.ReplaceAll(message.Text, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(out.String())
}

// header strips what would let a value break out of the header it is in.
func header(value string) string {
	return strings.NewReplacer("\r", "", "\n", "", "<", "", ">", "").Replace(value)
}

func portOr(port, fallback int) int {
	if port <= 0 || port > 65535 {
		return fallback
	}
	return port
}

// Link builds an absolute address for a path, for putting in a message.
func (m *Mailer) Link(path string) string {
	base := strings.TrimRight(m.settings.SiteURL(), "/")
	if base == "" {
		base = "http://localhost"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
