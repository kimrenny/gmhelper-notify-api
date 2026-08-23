package smtp

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gmhelper/notify-api/internal/app/email"
)

func TestBuildMIME_MultipartAlternative(t *testing.T) {
	from := "sender@example.com"
	to := "receiver@example.com"
	subject := "Test Subject"
	html := "<h1>Hello HTML</h1>"
	plain := "Hello Plain"

	raw := BuildMIME(from, to, subject, html, plain)
	rawStr := string(raw)

	if !strings.Contains(rawStr, "From: sender@example.com\r\n") {
		t.Errorf("missing From header: %s", rawStr)
	}
	if !strings.Contains(rawStr, "To: receiver@example.com\r\n") {
		t.Errorf("missing To header: %s", rawStr)
	}
	if !strings.Contains(rawStr, "Subject: Test Subject\r\n") {
		t.Errorf("missing Subject header: %s", rawStr)
	}
	if !strings.Contains(rawStr, "multipart/alternative") {
		t.Errorf("expected multipart/alternative, got: %s", rawStr)
	}
	if !strings.Contains(rawStr, "text/plain; charset=UTF-8") {
		t.Errorf("expected plain text part: %s", rawStr)
	}
	if !strings.Contains(rawStr, "text/html; charset=UTF-8") {
		t.Errorf("expected html part: %s", rawStr)
	}
}

func TestBuildMIME_HTMLOnly(t *testing.T) {
	raw := BuildMIME("from@example.com", "to@example.com", "Subject", "<p>Only HTML</p>", "")
	rawStr := string(raw)

	if !strings.Contains(rawStr, "Content-Type: text/html; charset=UTF-8") {
		t.Errorf("expected HTML Content-Type, got: %s", rawStr)
	}
	if strings.Contains(rawStr, "multipart/alternative") {
		t.Errorf("unexpected multipart/alternative in HTML-only email: %s", rawStr)
	}
}

func TestBuildMIME_PlainOnly(t *testing.T) {
	raw := BuildMIME("from@example.com", "to@example.com", "Subject", "", "Only Plain")
	rawStr := string(raw)

	if !strings.Contains(rawStr, "Content-Type: text/plain; charset=UTF-8") {
		t.Errorf("expected plain text Content-Type, got: %s", rawStr)
	}
	if strings.Contains(rawStr, "multipart/alternative") {
		t.Errorf("unexpected multipart/alternative in plain-only email: %s", rawStr)
	}
}

func TestBuildMIME_HeaderInjectionProtection(t *testing.T) {
	from := "sender@example.com\r\nBcc: evil@example.com"
	to := "receiver@example.com\nCc: victim@example.com"
	subject := "Subject\r\nX-Injected: value"

	raw := BuildMIME(from, to, subject, "<h1>Body</h1>", "Body")
	rawStr := string(raw)

	if strings.Contains(rawStr, "Bcc: evil@example.com") && strings.Contains(rawStr, "\r\nBcc:") {
		t.Errorf("header injection succeeded on From: %s", rawStr)
	}
	if strings.Contains(rawStr, "Cc: victim@example.com") && strings.Contains(rawStr, "\r\nCc:") {
		t.Errorf("header injection succeeded on To: %s", rawStr)
	}
	if strings.Contains(rawStr, "X-Injected: value") && strings.Contains(rawStr, "\r\nX-Injected:") {
		t.Errorf("header injection succeeded on Subject: %s", rawStr)
	}
}

func TestBuildMIME_NonASCIISubjectEncoding(t *testing.T) {
	subject := "Уведомление от GMHelper"
	raw := BuildMIME("from@example.com", "to@example.com", subject, "<h1>HTML</h1>", "Plain")
	rawStr := string(raw)

	if !strings.Contains(rawStr, "Subject: =?UTF-8?q?") && !strings.Contains(rawStr, "Subject: =?UTF-8?Q?") {
		t.Errorf("expected Q-encoded Subject header, got: %s", rawStr)
	}
}

func startFakeSMTPServer(t *testing.T, authRequired bool) (string, int, func()) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start fake smtp listener: %v", err)
	}

	addr := ln.Addr().(*net.TCPAddr)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleFakeSMTPConn(conn, authRequired)
		}
	}()

	return "127.0.0.1", addr.Port, func() {
		_ = ln.Close()
	}
}

func handleFakeSMTPConn(conn net.Conn, authRequired bool) {
	defer conn.Close()
	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	// 220 Greeting
	writer.WriteString("220 127.0.0.1 Fake SMTP Service Ready\r\n")
	writer.Flush()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO"):
			if authRequired {
				writer.WriteString("250-127.0.0.1 Hello\r\n250 AUTH PLAIN\r\n")
			} else {
				writer.WriteString("250 127.0.0.1 Hello\r\n")
			}
			writer.Flush()
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			writer.WriteString("235 2.7.0 Authentication successful\r\n")
			writer.Flush()
		case strings.HasPrefix(upper, "MAIL FROM:"):
			writer.WriteString("250 2.1.0 Sender OK\r\n")
			writer.Flush()
		case strings.HasPrefix(upper, "RCPT TO:"):
			if strings.Contains(upper, "REJECT@EXAMPLE.COM") {
				writer.WriteString("550 5.1.1 User unknown\r\n")
			} else {
				writer.WriteString("250 2.1.5 Recipient OK\r\n")
			}
			writer.Flush()
		case upper == "DATA":
			writer.WriteString("354 Start mail input; end with <CRLF>.<CRLF>\r\n")
			writer.Flush()
			// Read until single dot
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					writer.WriteString("250 2.0.0 OK message queued\r\n")
					writer.Flush()
					break
				}
			}
		case upper == "QUIT":
			writer.WriteString("221 2.0.0 Bye\r\n")
			writer.Flush()
			return
		default:
			writer.WriteString("500 Command unrecognized\r\n")
			writer.Flush()
		}
	}
}

func TestSMTPClient_Send_SuccessUnauthenticated(t *testing.T) {
	host, port, closeFn := startFakeSMTPServer(t, false)
	defer closeFn()

	client := NewClient(host, port, "", "", "no-reply@gmhelper.local")
	msg := &email.Message{
		To:            "user@example.com",
		Subject:       "Hello World",
		HTMLBody:      "<p>Test message</p>",
		PlainTextBody: "Test message",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Send(ctx, msg); err != nil {
		t.Fatalf("expected Send success, got: %v", err)
	}
}

func TestSMTPClient_Send_SuccessAuthenticated(t *testing.T) {
	host, port, closeFn := startFakeSMTPServer(t, true)
	defer closeFn()

	client := NewClient(host, port, "notify_user", "super_secret_password", "no-reply@gmhelper.local")
	msg := &email.Message{
		To:            "user@example.com",
		Subject:       "Authenticated Email",
		HTMLBody:      "<p>Secret content</p>",
		PlainTextBody: "Secret content",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Send(ctx, msg); err != nil {
		t.Fatalf("expected Send success with auth, got: %v", err)
	}
}

func TestSMTPClient_Send_RecipientRejected(t *testing.T) {
	host, port, closeFn := startFakeSMTPServer(t, false)
	defer closeFn()

	client := NewClient(host, port, "", "", "no-reply@gmhelper.local")
	msg := &email.Message{
		To:            "reject@example.com",
		Subject:       "Rejected Email",
		HTMLBody:      "<p>Rejected content</p>",
		PlainTextBody: "Rejected content",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := client.Send(ctx, msg)
	if err == nil {
		t.Fatal("expected error on rejected recipient, got nil")
	}

	if !strings.Contains(err.Error(), "SMTP RCPT command failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSMTPClient_Send_ConnectionRefusedNoPasswordLeak(t *testing.T) {
	// Pointing to a closed port
	secretPassword := "my_very_private_db_password_12345"
	client := NewClient("127.0.0.1", 65530, "user", secretPassword, "no-reply@gmhelper.local")

	msg := &email.Message{
		To:            "user@example.com",
		Subject:       "Connection failure test",
		HTMLBody:      "<p>Test</p>",
		PlainTextBody: "Test",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := client.Send(ctx, msg)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}

	if strings.Contains(err.Error(), secretPassword) {
		t.Fatalf("CRITICAL: sensitive password leaked in error message: %v", err)
	}
}
