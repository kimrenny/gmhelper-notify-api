package email

import "context"

type Message struct {
	From          string
	To            string
	Subject       string
	HTMLBody      string
	PlainTextBody string
}

type Sender interface {
	Send(ctx context.Context, message *Message) error
}
