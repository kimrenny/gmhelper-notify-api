package smtp

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/gmhelper/notify-api/internal/app/email"
)

type SMTPClient struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func NewSMTPClient(host string, port int, username, password, from string) *SMTPClient {
	return &SMTPClient{host: host, port: port, username: username, password: password, from: from}
}

type EmailMessage struct {
	To      string
	Subject string
	Body    string
}

func (c *SMTPClient) Send(ctx context.Context, message *email.Message) error {
	msg := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s", c.from, message.To, message.Subject, message.Body))
	addr := fmt.Sprintf("%s:%d", c.host, c.port)
	auth := smtp.PlainAuth("", c.username, c.password, c.host)
	return smtp.SendMail(addr, auth, c.from, []string{message.To}, msg)
}
