package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http/httptest"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type smtpCapture struct{ from, to, message string }
type smtpTestOptions struct {
	stall                  string
	ready, closed          chan struct{}
	rejectRCPT, rejectAuth bool
	tlsConfig              *tls.Config
}

func startTestSMTP(t *testing.T, opts smtpTestOptions) (string, <-chan smtpCapture) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	captures := make(chan smtpCapture, 10)
	var mu sync.Mutex
	conns := make(map[net.Conn]bool)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns[conn] = true
			mu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { conn.Close(); mu.Lock(); delete(conns, conn); mu.Unlock() }()
				serveTestSMTP(conn, opts, captures)
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		for conn := range conns {
			conn.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	return listener.Addr().String(), captures
}

func serveTestSMTP(conn net.Conn, opts smtpTestOptions, captures chan<- smtpCapture) {
	stall := func() {
		close(opts.ready)
		_, _ = io.Copy(io.Discard, conn)
		close(opts.closed)
	}
	if opts.stall == "banner" {
		stall()
		return
	}
	text := textproto.NewConn(conn)
	if text.PrintfLine("220 localhost ESMTP") != nil {
		return
	}
	var capture smtpCapture
	for {
		line, err := text.ReadLine()
		if err != nil {
			return
		}
		switch {
		case strings.HasPrefix(line, "EHLO"):
			if opts.tlsConfig != nil {
				_ = text.PrintfLine("250-localhost\r\n250 STARTTLS")
			} else {
				_ = text.PrintfLine("250-localhost\r\n250 AUTH PLAIN")
			}
		case line == "STARTTLS":
			_ = text.PrintfLine("220 Ready for TLS")
			tlsConn := tls.Server(conn, opts.tlsConfig)
			if tlsConn.Handshake() != nil {
				return
			}
			text = textproto.NewConn(tlsConn)
		case strings.HasPrefix(line, "AUTH"):
			if opts.rejectAuth {
				_ = text.PrintfLine("535 Authentication rejected")
			} else {
				_ = text.PrintfLine("235 Authentication accepted")
			}
		case strings.HasPrefix(line, "MAIL FROM:"):
			capture.from = strings.TrimPrefix(line, "MAIL FROM:")
			_ = text.PrintfLine("250 Sender OK")
		case strings.HasPrefix(line, "RCPT TO:"):
			if opts.rejectRCPT {
				_ = text.PrintfLine("550 Recipient rejected")
			} else {
				capture.to = strings.TrimPrefix(line, "RCPT TO:")
				_ = text.PrintfLine("250 Recipient OK")
			}
		case line == "DATA":
			_ = text.PrintfLine("354 Send message")
			body, err := text.ReadDotBytes()
			if err != nil {
				return
			}
			capture.message = string(body)
			if opts.stall == "data" {
				stall()
				return
			}
			captures <- capture
			_ = text.PrintfLine("250 Accepted")
		case line == "QUIT":
			_ = text.PrintfLine("221 Bye")
			return
		default:
			_ = text.PrintfLine("250 OK")
		}
	}
}

func TestSendMailContextCancellation(t *testing.T) {
	t.Run("before dial", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := sendMailContext(ctx, "127.0.0.1:1", nil, "a@example.com", []string{"b@example.com"}, []byte("test"))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})
	for _, stage := range []string{"banner", "data"} {
		t.Run(stage, func(t *testing.T) {
			ready, closed := make(chan struct{}), make(chan struct{})
			addr, _ := startTestSMTP(t, smtpTestOptions{stall: stage, ready: ready, closed: closed})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- sendMailContext(ctx, addr, nil, "a@example.com", []string{"b@example.com"}, []byte("Subject: test\r\n\r\nbody"))
			}()
			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				t.Fatal("SMTP did not reach stalled stage")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error = %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancel did not interrupt SMTP")
			}
			select {
			case <-closed:
			case <-time.After(3 * time.Second):
				t.Fatal("connection left open")
			}
		})
	}
}

func TestSendMailContextDeadline(t *testing.T) {
	addr, _ := startTestSMTP(t, smtpTestOptions{stall: "banner", ready: make(chan struct{}), closed: make(chan struct{})})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := sendMailContext(ctx, addr, nil, "a@example.com", []string{"b@example.com"}, []byte("test"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v", err)
	}
}

func TestSendMailContextRejectsSMTPFailures(t *testing.T) {
	t.Run("recipient", func(t *testing.T) {
		addr, captures := startTestSMTP(t, smtpTestOptions{rejectRCPT: true})
		err := sendMailContext(context.Background(), addr, nil, "a@example.com", []string{"b@example.com"}, []byte("test"))
		if err == nil || !strings.Contains(err.Error(), "550") {
			t.Fatalf("error = %v", err)
		}
		select {
		case <-captures:
			t.Fatal("rejected message submitted")
		default:
		}
	})
	t.Run("authentication", func(t *testing.T) {
		addr, _ := startTestSMTP(t, smtpTestOptions{rejectAuth: true})
		err := sendMailContext(context.Background(), addr, smtp.PlainAuth("", "test", "test", "127.0.0.1"), "a@example.com", []string{"b@example.com"}, []byte("test"))
		if err == nil || !strings.Contains(err.Error(), "535") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("untrusted TLS", func(t *testing.T) {
		https := httptest.NewTLSServer(nil)
		defer https.Close()
		addr, captures := startTestSMTP(t, smtpTestOptions{tlsConfig: https.TLS})
		err := sendMailContext(context.Background(), addr, nil, "a@example.com", []string{"b@example.com"}, []byte("test"))
		if err == nil {
			t.Fatal("untrusted certificate accepted")
		}
		select {
		case <-captures:
			t.Fatal("message sent after TLS failure")
		default:
		}
	})
}

func TestBrevoSenderUsesConfiguredFrom(t *testing.T) {
	addr, captures := startTestSMTP(t, smtpTestOptions{})
	host, rawPort, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(rawPort)
	base := BrevoSender{Host: host, Port: port, From: "Default <default@example.com>"}
	var wg sync.WaitGroup
	for _, from := range []string{"default@example.com", "support@example.com", "billing@example.com"} {
		sender := base
		if from != "default@example.com" {
			sender.From = fmt.Sprintf("Team <%s>", from)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sender.SendEmail(context.Background(), EmailPayload{Type: "notification", To: []string{"b@example.com"}, Subject: "Hello", TextBody: "body"}); err != nil {
				t.Errorf("send: %v", err)
			}
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for range 3 {
		select {
		case capture := <-captures:
			message, err := mail.ReadMessage(strings.NewReader(capture.message))
			if err != nil {
				t.Fatal(err)
			}
			from, err := mail.ParseAddress(message.Header.Get("From"))
			if err != nil {
				t.Fatal(err)
			}
			if capture.from != "<"+from.Address+">" || capture.to != "<b@example.com>" || message.Header.Get("Subject") != "[NOTIFICATION] Hello" {
				t.Fatalf("unexpected delivery: %+v", capture)
			}
			seen[from.Address] = true
		case <-time.After(3 * time.Second):
			t.Fatal("delivery missing")
		}
	}
	if len(seen) != 3 || !seen["default@example.com"] || !seen["support@example.com"] || !seen["billing@example.com"] || base.From != "Default <default@example.com>" {
		t.Fatalf("senders = %v", seen)
	}
}

func TestBrevoSenderEnvelopeMatchesQuotedDisplayName(t *testing.T) {
	addr, captures := startTestSMTP(t, smtpTestOptions{})
	host, rawPort, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(rawPort)
	sender := BrevoSender{Host: host, Port: port, From: `"Support <Ops>" <support@example.com>`}
	if err := sender.SendEmail(context.Background(), EmailPayload{To: []string{"a@example.com"}, Subject: "Hello", TextBody: "Body"}); err != nil {
		t.Fatal(err)
	}
	select {
	case capture := <-captures:
		if capture.from != "<support@example.com>" {
			t.Fatalf("envelope From = %q, want verified mailbox", capture.from)
		}
	case <-time.After(time.Second):
		t.Fatal("message not received")
	}
}

func TestExtractEmailPreservesDefaultFromFormats(t *testing.T) {
	for _, input := range []string{"default@example.com", "Default <default@example.com>", `"Support <Ops>" <default@example.com>`} {
		if got := extractEmail(input); got != "default@example.com" {
			t.Fatalf("extract %q = %q", input, got)
		}
	}
}

func TestBuildGeneralMessagePreservesAttachmentBytes(t *testing.T) {
	want := append([]byte{0x00, 0xff, 0x50, 0x44, 0x46}, bytes.Repeat([]byte{0xab}, 128)...)
	messageData, err := buildGeneralMessage("Sender <sender@example.com>", "recipient@example.com", EmailPayload{
		Subject: "[NOTIFICATION] CV", TextBody: "Plain body", HTMLBody: "<p>HTML body</p>",
		Attachments: []EmailAttachment{{Name: "resume.pdf", Content: want}},
	})
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(messageData))
	if err != nil {
		t.Fatal(err)
	}
	mediaType, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("outer content type = %q, %v", mediaType, err)
	}
	outer := multipart.NewReader(message.Body, params["boundary"])
	alternative, err := outer.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	alternativeType, alternativeParams, err := mime.ParseMediaType(alternative.Header.Get("Content-Type"))
	if err != nil || alternativeType != "multipart/alternative" {
		t.Fatalf("alternative content type = %q, %v", alternativeType, err)
	}
	inner := multipart.NewReader(alternative, alternativeParams["boundary"])
	textPart, err := inner.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	textBody, err := io.ReadAll(textPart)
	if err != nil || string(textBody) != "Plain body" {
		t.Fatalf("text body = %q, %v", textBody, err)
	}
	htmlPart, err := inner.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	htmlBody, err := io.ReadAll(htmlPart)
	if err != nil || string(htmlBody) != "<p>HTML body</p>" {
		t.Fatalf("HTML body = %q, %v", htmlBody, err)
	}
	attachment, err := outer.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	disposition, dispositionParams, err := mime.ParseMediaType(attachment.Header.Get("Content-Disposition"))
	if err != nil || disposition != "attachment" || dispositionParams["filename"] != "resume.pdf" {
		t.Fatalf("attachment disposition = %q, %v", attachment.Header.Get("Content-Disposition"), err)
	}
	if attachment.Header.Get("Content-Transfer-Encoding") != "base64" {
		t.Fatalf("attachment transfer encoding = %q", attachment.Header.Get("Content-Transfer-Encoding"))
	}
	encoded, err := io.ReadAll(attachment)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(encoded), "\r\n"), "\r\n") {
		if len(line) > 76 {
			t.Fatalf("Base64 line has %d characters", len(line))
		}
	}
	got, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(encoded), "\r\n", ""))
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("attachment bytes = %v, %v; want %v", got, err, want)
	}
}
