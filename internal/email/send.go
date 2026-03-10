package email

import (
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/iley/mailfeed/internal/config"
	"github.com/iley/mailfeed/internal/feed"
)

type Sender struct {
	cfg config.Email
}

func NewSender(cfg config.Email) *Sender {
	return &Sender{cfg: cfg}
}

// SendAll sends one email per item over a single SMTP connection.
// The onSent callback is called after each item is successfully sent,
// allowing the caller to save progress incrementally.
func (s *Sender) SendAll(items []feed.Item, onSent func(feed.Item)) error {
	if len(items) == 0 {
		return nil
	}

	c, err := s.connect()
	if err != nil {
		return fmt.Errorf("smtp connect: %w", err)
	}
	defer c.Close()

	if err := s.authenticate(c); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}

	var errs []string
	for _, item := range items {
		if err := s.sendItem(c, item); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", item.Title, err))
			// Try to reset the connection for the next item.
			if rErr := c.Reset(); rErr != nil {
				// Connection is broken, no point continuing.
				return fmt.Errorf("sending failed (%d errors), connection lost: %s", len(errs), strings.Join(errs, "; "))
			}
			continue
		}
		if onSent != nil {
			onSent(item)
		}
	}

	c.Quit()

	if len(errs) > 0 {
		return fmt.Errorf("failed to send %d items: %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

func (s *Sender) useImplicitTLS() bool {
	switch s.cfg.SMTP.TLS {
	case "implicit":
		return true
	case "starttls":
		return false
	default:
		return s.cfg.SMTP.Port == 465
	}
}

const (
	smtpDialTimeout    = 30 * time.Second
	smtpOverallTimeout = 5 * time.Minute
)

func (s *Sender) connect() (*smtp.Client, error) {
	addr := net.JoinHostPort(s.cfg.SMTP.Host, fmt.Sprintf("%d", s.cfg.SMTP.Port))
	tlsCfg := &tls.Config{ServerName: s.cfg.SMTP.Host}
	dialer := &net.Dialer{Timeout: smtpDialTimeout}

	if s.useImplicitTLS() {
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
		if err != nil {
			return nil, err
		}
		conn.SetDeadline(time.Now().Add(smtpOverallTimeout))
		c, err := smtp.NewClient(conn, s.cfg.SMTP.Host)
		if err != nil {
			conn.Close()
			return nil, err
		}
		return c, nil
	}

	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(smtpOverallTimeout))
	c, err := smtp.NewClient(conn, s.cfg.SMTP.Host)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := c.StartTLS(tlsCfg); err != nil {
		c.Close()
		return nil, fmt.Errorf("starttls: %w", err)
	}
	return c, nil
}

func (s *Sender) authenticate(c *smtp.Client) error {
	if s.cfg.SMTP.Username == "" {
		return nil
	}
	auth := smtp.PlainAuth("", s.cfg.SMTP.Username, s.cfg.SMTP.Password, s.cfg.SMTP.Host)
	return c.Auth(auth)
}

func (s *Sender) sendItem(c *smtp.Client, item feed.Item) error {
	msg, err := buildMessage(s.cfg.From, s.cfg.To, item)
	if err != nil {
		return err
	}

	if err := c.Mail(s.cfg.From); err != nil {
		return err
	}
	if err := c.Rcpt(s.cfg.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(msg)); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func buildMessage(from, to string, item feed.Item) (string, error) {
	htmlBody, err := RenderHTML(item)
	if err != nil {
		return "", fmt.Errorf("render html: %w", err)
	}
	textBody, err := RenderPlainText(item)
	if err != nil {
		return "", fmt.Errorf("render text: %w", err)
	}

	subject := item.Title
	if item.FeedName != "" {
		subject = fmt.Sprintf("[%s] %s", item.FeedName, item.Title)
	}

	boundary := fmt.Sprintf("mailfeed-%d", time.Now().UnixNano())
	msgID := fmt.Sprintf("<%d.mailfeed@%s>", time.Now().UnixNano(), hostFromAddr(from))

	var b strings.Builder
	// Headers
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: %s\r\n", msgID)
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n", boundary)
	fmt.Fprintf(&b, "\r\n")

	// Plain text part
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=utf-8\r\n")
	fmt.Fprintf(&b, "Content-Transfer-Encoding: 8bit\r\n")
	fmt.Fprintf(&b, "\r\n")
	b.WriteString(textBody)
	fmt.Fprintf(&b, "\r\n")

	// HTML part
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprintf(&b, "Content-Type: text/html; charset=utf-8\r\n")
	fmt.Fprintf(&b, "Content-Transfer-Encoding: 8bit\r\n")
	fmt.Fprintf(&b, "\r\n")
	b.WriteString(htmlBody)
	fmt.Fprintf(&b, "\r\n")

	// Closing boundary
	fmt.Fprintf(&b, "--%s--\r\n", boundary)

	return b.String(), nil
}

func hostFromAddr(email string) string {
	_, host, ok := strings.Cut(email, "@")
	if !ok {
		host, _, _ = net.SplitHostPort(email)
		if host == "" {
			return "localhost"
		}
	}
	return host
}
