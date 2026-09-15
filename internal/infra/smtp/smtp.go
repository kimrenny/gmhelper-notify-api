package smtp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/gmhelper/notify-api/internal/app/email"
	"github.com/gmhelper/notify-api/internal/infra/logger"
)

type Client struct {
	host     string
	port     int
	username string
	password string
	from     string
	logger   logger.Logger
}

func NewClient(host string, port int, username, password, from string) *Client {
	return NewClientWithLogger(host, port, username, password, from, nil)
}

func NewClientWithLogger(host string, port int, username, password, from string, log logger.Logger) *Client {
	return &Client{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     strings.TrimSpace(from),
		logger:   log,
	}
}

func (c *Client) Send(ctx context.Context, message *email.Message) error {
	if message == nil {
		return errors.New("cannot send nil email message")
	}
	to := sanitizeHeader(message.To)
	if to == "" {
		return errors.New("recipient email address is required")
	}
	from := sanitizeHeader(message.From)
	if from == "" {
		from = c.from
	}
	if from == "" {
		return errors.New("sender email address (From) is required")
	}

	rawMsg := BuildMIME(from, to, message.Subject, message.HTMLBody, message.PlainTextBody)

	if c.logger != nil {
		c.logger.Info("connecting to SMTP server for email delivery",
			logger.String("smtpHost", c.host),
			logger.Int("smtpPort", c.port),
			logger.String("from", from),
			logger.String("to", to),
			logger.String("subject", message.Subject),
		)
	}

	hostPort := net.JoinHostPort(c.host, strconv.Itoa(c.port))
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("failed to connect to SMTP server",
				logger.String("smtpHost", c.host),
				logger.Int("smtpPort", c.port),
				logger.String("from", from),
				logger.String("to", to),
				logger.Error(err),
			)
		}
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	smtpClient, err := smtp.NewClient(conn, c.host)
	if err != nil {
		if c.logger != nil {
			c.logger.Error("failed to initialize SMTP client handshake",
				logger.String("smtpHost", c.host),
				logger.Int("smtpPort", c.port),
				logger.Error(err),
			)
		}
		return fmt.Errorf("failed to initialize SMTP client: %w", err)
	}
	defer smtpClient.Close()

	if ok, _ := smtpClient.Extension("STARTTLS"); ok {
		tlsConfig := &tls.Config{
			ServerName: c.host,
		}
		if err := smtpClient.StartTLS(tlsConfig); err != nil {
			if c.logger != nil {
				c.logger.Error("SMTP STARTTLS negotiation failed",
					logger.String("smtpHost", c.host),
					logger.Int("smtpPort", c.port),
					logger.Error(err),
				)
			}
			return fmt.Errorf("SMTP STARTTLS negotiation failed: %w", err)
		}
	}

	if c.username != "" || c.password != "" {
		auth := smtp.PlainAuth("", c.username, c.password, c.host)
		if err := smtpClient.Auth(auth); err != nil {
			if c.logger != nil {
				c.logger.Error("SMTP authentication failed",
					logger.String("smtpHost", c.host),
					logger.Int("smtpPort", c.port),
					logger.String("username", c.username),
					logger.Error(err),
				)
			}
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	if err := smtpClient.Mail(from); err != nil {
		if c.logger != nil {
			c.logger.Error("SMTP MAIL command failed",
				logger.String("smtpHost", c.host),
				logger.Int("smtpPort", c.port),
				logger.String("from", from),
				logger.Error(err),
			)
		}
		return fmt.Errorf("SMTP MAIL command failed: %w", err)
	}

	if err := smtpClient.Rcpt(to); err != nil {
		if c.logger != nil {
			c.logger.Error("SMTP RCPT command failed",
				logger.String("smtpHost", c.host),
				logger.Int("smtpPort", c.port),
				logger.String("to", to),
				logger.Error(err),
			)
		}
		return fmt.Errorf("SMTP RCPT command failed: %w", err)
	}

	w, err := smtpClient.Data()
	if err != nil {
		if c.logger != nil {
			c.logger.Error("SMTP DATA command initialization failed",
				logger.String("smtpHost", c.host),
				logger.Int("smtpPort", c.port),
				logger.Error(err),
			)
		}
		return fmt.Errorf("SMTP DATA command failed: %w", err)
	}

	if _, err := w.Write(rawMsg); err != nil {
		_ = w.Close()
		if c.logger != nil {
			c.logger.Error("failed to write email payload to SMTP stream",
				logger.String("smtpHost", c.host),
				logger.Int("smtpPort", c.port),
				logger.Error(err),
			)
		}
		return fmt.Errorf("failed to write email body: %w", err)
	}

	if err := w.Close(); err != nil {
		if c.logger != nil {
			c.logger.Error("failed to finalize SMTP DATA command",
				logger.String("smtpHost", c.host),
				logger.Int("smtpPort", c.port),
				logger.Error(err),
			)
		}
		return fmt.Errorf("failed to finalize SMTP DATA command: %w", err)
	}

	if err := smtpClient.Quit(); err != nil {
		if c.logger != nil {
			c.logger.Warn("SMTP QUIT command returned error (message already sent)",
				logger.String("smtpHost", c.host),
				logger.Int("smtpPort", c.port),
				logger.Error(err),
			)
		}
	}

	if c.logger != nil {
		c.logger.Info("SMTP message accepted successfully by server",
			logger.String("smtpHost", c.host),
			logger.Int("smtpPort", c.port),
			logger.String("from", from),
			logger.String("to", to),
		)
	}

	return nil
}

// BuildMIME constructs a compliant MIME email message bytes.
func BuildMIME(from, to, subject, htmlBody, plainTextBody string) []byte {
	from = sanitizeHeader(from)
	to = sanitizeHeader(to)
	subject = encodeSubject(subject)

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("From: %s\r\n", from))
	buf.WriteString(fmt.Sprintf("To: %s\r\n", to))
	buf.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	buf.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z)))

	msgDomain := "gmhelper.local"
	if atIdx := strings.LastIndex(from, "@"); atIdx != -1 && atIdx+1 < len(from) {
		msgDomain = from[atIdx+1:]
	}
	buf.WriteString(fmt.Sprintf("Message-ID: <%d.%s@%s>\r\n", time.Now().UnixNano(), generateBoundary(), msgDomain))
	buf.WriteString("MIME-Version: 1.0\r\n")

	hasHTML := strings.TrimSpace(htmlBody) != ""
	hasPlain := strings.TrimSpace(plainTextBody) != ""

	switch {
	case hasHTML && hasPlain:
		boundary := generateBoundary()
		buf.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n", boundary))

		// Plain text part
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		buf.WriteString(plainTextBody)
		buf.WriteString("\r\n\r\n")

		// HTML part
		buf.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		buf.WriteString(htmlBody)
		buf.WriteString("\r\n\r\n")

		buf.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	case hasHTML && !hasPlain:
		buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		buf.WriteString(htmlBody)
		buf.WriteString("\r\n")

	default:
		buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		buf.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		buf.WriteString(plainTextBody)
		buf.WriteString("\r\n")
	}

	return buf.Bytes()
}

func sanitizeHeader(val string) string {
	val = strings.ReplaceAll(val, "\r", "")
	val = strings.ReplaceAll(val, "\n", "")
	return strings.TrimSpace(val)
}

func encodeSubject(subject string) string {
	subject = sanitizeHeader(subject)
	for i := 0; i < len(subject); i++ {
		if subject[i] > 127 {
			return mime.QEncoding.Encode("UTF-8", subject)
		}
	}
	return subject
}

func generateBoundary() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("boundary_%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("mime_boundary_%s", hex.EncodeToString(b))
}
