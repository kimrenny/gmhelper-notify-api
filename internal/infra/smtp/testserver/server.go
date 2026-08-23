package testserver

import (
	"bufio"
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
)

type ReceivedMessage struct {
	From     string
	To       []string
	Raw      []byte
	Subject  string
	TextBody string
	HTMLBody string
	Headers  mail.Header
}

type FakeSMTPServer struct {
	Addr            string
	Host            string
	Port            int
	listener        net.Listener
	mu              sync.Mutex
	messages        []ReceivedMessage
	RejectRecipient bool
	RejectData      bool
	closed          bool
}

func StartFakeSMTPServer(t *testing.T) *FakeSMTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start fake smtp listener: %v", err)
	}

	tcpAddr := ln.Addr().(*net.TCPAddr)
	server := &FakeSMTPServer{
		Addr:     ln.Addr().String(),
		Host:     "127.0.0.1",
		Port:     tcpAddr.Port,
		listener: ln,
	}

	go server.serve()
	return server
}

func (s *FakeSMTPServer) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	if s.listener != nil {
		_ = s.listener.Close()
	}
}

func (s *FakeSMTPServer) GetMessages() []ReceivedMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]ReceivedMessage, len(s.messages))
	copy(result, s.messages)
	return result
}

func (s *FakeSMTPServer) ResetMessages() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = nil
}

func (s *FakeSMTPServer) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *FakeSMTPServer) handleConn(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// Send initial greeting
	writer.WriteString("220 127.0.0.1 Fake SMTP Service Ready\r\n")
	writer.Flush()

	var currentFrom string
	var currentTo []string
	var inData bool
	var dataBuffer bytes.Buffer

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		if inData {
			if strings.TrimRight(line, "\r\n") == "." {
				inData = false
				s.mu.Lock()
				rejectData := s.RejectData
				s.mu.Unlock()

				if rejectData {
					writer.WriteString("554 5.3.0 Transaction failed\r\n")
					writer.Flush()
					dataBuffer.Reset()
					continue
				}

				rawBytes := dataBuffer.Bytes()
				parsedMsg := parseMIMEMessage(currentFrom, currentTo, rawBytes)

				s.mu.Lock()
				s.messages = append(s.messages, parsedMsg)
				s.mu.Unlock()

				writer.WriteString("250 2.0.0 OK message queued\r\n")
				writer.Flush()
				dataBuffer.Reset()
				currentFrom = ""
				currentTo = nil
				continue
			}

			dataBuffer.WriteString(line)
			continue
		}

		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)

		switch {
		case strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO"):
			writer.WriteString("250-127.0.0.1 Hello\r\n250 AUTH PLAIN\r\n")
			writer.Flush()
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			writer.WriteString("235 2.7.0 Authentication successful\r\n")
			writer.Flush()
		case strings.HasPrefix(upper, "MAIL FROM:"):
			currentFrom = extractEmailFromCommand(trimmed[10:])
			writer.WriteString("250 2.1.0 Sender OK\r\n")
			writer.Flush()
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.mu.Lock()
			rejectRecipient := s.RejectRecipient
			s.mu.Unlock()

			if rejectRecipient {
				writer.WriteString("550 5.1.1 User unknown\r\n")
				writer.Flush()
				continue
			}
			recipient := extractEmailFromCommand(trimmed[8:])
			currentTo = append(currentTo, recipient)
			writer.WriteString("250 2.1.5 Recipient OK\r\n")
			writer.Flush()
		case upper == "DATA":
			inData = true
			dataBuffer.Reset()
			writer.WriteString("354 Start mail input; end with <CRLF>.<CRLF>\r\n")
			writer.Flush()
		case upper == "RSET":
			currentFrom = ""
			currentTo = nil
			dataBuffer.Reset()
			writer.WriteString("250 2.0.0 Reset OK\r\n")
			writer.Flush()
		case upper == "NOOP":
			writer.WriteString("250 2.0.0 OK\r\n")
			writer.Flush()
		case upper == "QUIT":
			writer.WriteString("221 2.0.0 Bye\r\n")
			writer.Flush()
			return
		default:
			writer.WriteString("500 5.5.1 Command unrecognized\r\n")
			writer.Flush()
		}
	}
}

func extractEmailFromCommand(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	return strings.TrimSpace(s)
}

func parseMIMEMessage(from string, to []string, raw []byte) ReceivedMessage {
	msg := ReceivedMessage{
		From: from,
		To:   to,
		Raw:  raw,
	}

	mailMsg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return msg
	}

	msg.Headers = mailMsg.Header
	msg.Subject = decodeMIMEHeader(mailMsg.Header.Get("Subject"))

	mediaType, params, err := mime.ParseMediaType(mailMsg.Header.Get("Content-Type"))
	if err != nil {
		// Fallback to reading body as text
		bodyBytes, _ := io.ReadAll(mailMsg.Body)
		msg.TextBody = string(bodyBytes)
		return msg
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			mr := multipart.NewReader(mailMsg.Body, boundary)
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				partMediaType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
				partBytes, _ := io.ReadAll(part)
				if strings.HasPrefix(partMediaType, "text/plain") {
					msg.TextBody = string(partBytes)
				} else if strings.HasPrefix(partMediaType, "text/html") {
					msg.HTMLBody = string(partBytes)
				}
			}
		}
	} else if strings.HasPrefix(mediaType, "text/html") {
		bodyBytes, _ := io.ReadAll(mailMsg.Body)
		msg.HTMLBody = string(bodyBytes)
	} else {
		bodyBytes, _ := io.ReadAll(mailMsg.Body)
		msg.TextBody = string(bodyBytes)
	}

	return msg
}

func decodeMIMEHeader(input string) string {
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(input)
	if err != nil {
		return input
	}
	return decoded
}
