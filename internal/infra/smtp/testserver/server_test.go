package testserver

import (
	"context"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/email"
	infrasmtp "github.com/gmhelper/notify-api/internal/infra/smtp"
)

func TestFakeSMTPServer_CaptureMultipartEmail(t *testing.T) {
	server := StartFakeSMTPServer(t)
	defer server.Close()

	client := infrasmtp.NewClient(server.Host, server.Port, "", "", "sender@example.com")
	msg := &email.Message{
		To:            "recipient@example.com",
		Subject:       "Welcome to GMHelper",
		PlainTextBody: "Hello, welcome to GMHelper!",
		HTMLBody:      "<h1>Hello</h1><p>Welcome to GMHelper!</p>",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Send(ctx, msg); err != nil {
		t.Fatalf("expected client.Send success, got: %v", err)
	}

	messages := server.GetMessages()
	if len(messages) != 1 {
		t.Fatalf("expected 1 captured message, got %d", len(messages))
	}

	captured := messages[0]
	if captured.From != "sender@example.com" {
		t.Errorf("expected From sender@example.com, got %s", captured.From)
	}
	if len(captured.To) != 1 || captured.To[0] != "recipient@example.com" {
		t.Errorf("expected To recipient@example.com, got %v", captured.To)
	}
	if captured.Subject != "Welcome to GMHelper" {
		t.Errorf("expected Subject 'Welcome to GMHelper', got %s", captured.Subject)
	}
	if !strings.Contains(captured.TextBody, "Hello, welcome to GMHelper!") {
		t.Errorf("expected TextBody content, got: %s", captured.TextBody)
	}
	if !strings.Contains(captured.HTMLBody, "<h1>Hello</h1>") {
		t.Errorf("expected HTMLBody content, got: %s", captured.HTMLBody)
	}
}

func TestFakeSMTPServer_RejectRecipient(t *testing.T) {
	server := StartFakeSMTPServer(t)
	defer server.Close()
	server.RejectRecipient = true

	addr := server.Addr
	err := smtp.SendMail(addr, nil, "sender@example.com", []string{"bad@example.com"}, []byte("Subject: Hi\r\n\r\nBody"))
	if err == nil {
		t.Fatal("expected SendMail error on rejected recipient, got nil")
	}
}
