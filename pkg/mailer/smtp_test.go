package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
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
	if opts.stall == "banner" {
		stallTestSMTP(conn, opts)
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
		var keepGoing bool
		text, keepGoing = serveTestSMTPCommand(text, conn, opts, captures, &capture, line)
		if !keepGoing {
			return
		}
	}
}

func stallTestSMTP(conn net.Conn, opts smtpTestOptions) {
	close(opts.ready)
	_, _ = io.Copy(io.Discard, conn)
	close(opts.closed)
}

func serveTestSMTPCommand(text *textproto.Conn, conn net.Conn, opts smtpTestOptions, captures chan<- smtpCapture, capture *smtpCapture, line string) (*textproto.Conn, bool) {
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
			return text, false
		}
		return textproto.NewConn(tlsConn), true
	case strings.HasPrefix(line, "AUTH"):
		replyTestSMTPAuth(text, opts.rejectAuth)
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
		if !readTestSMTPData(text, conn, opts, captures, capture) {
			return text, false
		}
	case line == "QUIT":
		_ = text.PrintfLine("221 Bye")
		return text, false
	default:
		_ = text.PrintfLine("250 OK")
	}
	return text, true
}

func replyTestSMTPAuth(text *textproto.Conn, reject bool) {
	if reject {
		_ = text.PrintfLine("535 Authentication rejected")
		return
	}
	_ = text.PrintfLine("235 Authentication accepted")
}

func readTestSMTPData(text *textproto.Conn, conn net.Conn, opts smtpTestOptions, captures chan<- smtpCapture, capture *smtpCapture) bool {
	_ = text.PrintfLine("354 Send message")
	body, err := text.ReadDotBytes()
	if err != nil {
		return false
	}
	capture.message = string(body)
	if opts.stall == "data" {
		stallTestSMTP(conn, opts)
		return false
	}
	captures <- *capture
	_ = text.PrintfLine("250 Accepted")
	return true
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
			assertSendMailCancellationAt(t, stage)
		})
	}
}

func assertSendMailCancellationAt(t *testing.T, stage string) {
	t.Helper()
	ready, closed := make(chan struct{}), make(chan struct{})
	options := smtpTestOptions{stall: stage, ready: ready, closed: closed}
	send := func(ctx context.Context, addr string) error {
		return sendMailContext(ctx, addr, nil, "a@example.com", []string{"b@example.com"}, []byte("Subject: test\r\n\r\nbody"))
	}
	if stage == "data" {
		serverTLS, clientTLS := testSMTPTLSConfigs(t)
		options.tlsConfig = serverTLS
		send = func(ctx context.Context, addr string) error {
			return sendMailContextWithTLSConfig(ctx, addr, nil, "a@example.com", []string{"b@example.com"}, []byte("Subject: test\r\n\r\nbody"), clientTLS)
		}
	}
	addr, _ := startTestSMTP(t, options)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- send(ctx, addr) }()
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
		serverTLS, clientTLS := testSMTPTLSConfigs(t)
		addr, captures := startTestSMTP(t, smtpTestOptions{rejectRCPT: true, tlsConfig: serverTLS})
		err := sendMailContextWithTLSConfig(context.Background(), addr, nil, "a@example.com", []string{"b@example.com"}, []byte("test"), clientTLS)
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
		serverTLS, clientTLS := testSMTPTLSConfigs(t)
		addr, _ := startTestSMTP(t, smtpTestOptions{rejectAuth: true, tlsConfig: serverTLS})
		err := sendMailContextWithTLSConfig(context.Background(), addr, smtp.PlainAuth("", "test", "test", "127.0.0.1"), "a@example.com", []string{"b@example.com"}, []byte("test"), clientTLS)
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

func testSMTPTLSConfigs(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	server := httptest.NewTLSServer(nil)
	defer server.Close()
	cert, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return server.TLS, &tls.Config{RootCAs: roots}
}

func TestSendMailContextRequiresSTARTTLS(t *testing.T) {
	addr, captures := startTestSMTP(t, smtpTestOptions{})
	err := sendMailContext(context.Background(), addr, nil, "a@example.com", []string{"b@example.com"}, []byte("test"))
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("error = %v, want missing STARTTLS", err)
	}
	select {
	case <-captures:
		t.Fatal("message sent without TLS")
	default:
	}
}

func TestBrevoSenderUsesConfiguredFrom(t *testing.T) {
	base := BrevoSender{From: "Default <default@example.com>"}
	for _, from := range []string{"default@example.com", "support@example.com", "billing@example.com"} {
		wantAddress := from
		sender := base
		if from != "default@example.com" {
			sender.From = fmt.Sprintf("Team <%s>", from)
		}
		message, err := buildGeneralMessage(sender.From, "b@example.com", EmailPayload{Type: "notification", Subject: "[NOTIFICATION] Hello", TextBody: "body"})
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := mail.ReadMessage(strings.NewReader(string(message)))
		if err != nil {
			t.Fatal(err)
		}
		parsedFrom, err := mail.ParseAddress(parsed.Header.Get("From"))
		if err != nil || extractEmail(sender.From) != parsedFrom.Address || parsedFrom.Address != wantAddress {
			t.Fatalf("configured From = %q, parsed %q, error %v", sender.From, parsedFrom, err)
		}
	}
	if base.From != "Default <default@example.com>" {
		t.Fatalf("base sender mutated: %q", base.From)
	}
}

func TestBrevoSenderEnvelopeMatchesQuotedDisplayName(t *testing.T) {
	sender := BrevoSender{From: `"Support <Ops>" <support@example.com>`}
	messageData, err := buildGeneralMessage(sender.From, "a@example.com", EmailPayload{Subject: "Hello", TextBody: "Body"})
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(messageData))
	if err != nil {
		t.Fatal(err)
	}
	from, err := mail.ParseAddress(message.Header.Get("From"))
	if err != nil || extractEmail(sender.From) != from.Address || from.Address != "support@example.com" {
		t.Fatalf("envelope From = %q, want verified mailbox; error %v", from, err)
	}
}

func assertSMTPSenderContextCancellation(t *testing.T, send func(context.Context, *BrevoSender) error) {
	t.Helper()
	ready, closed := make(chan struct{}), make(chan struct{})
	addr, _ := startTestSMTP(t, smtpTestOptions{stall: "banner", ready: ready, closed: closed})
	host, rawPort, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}
	sender := &BrevoSender{Host: host, Port: port, User: "user", Pass: "pass", From: "sender@example.com", TTL: 5 * time.Minute}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- send(ctx, sender) }()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("SMTP did not reach stalled banner")
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
}

func TestBrevoSenderSendOTPUsesContext(t *testing.T) {
	assertSMTPSenderContextCancellation(t, func(ctx context.Context, sender *BrevoSender) error {
		return sender.SendOTP(ctx, "to@example.com", "123456", "App")
	})
}

func TestBrevoSenderSendPasswordResetUsesContext(t *testing.T) {
	assertSMTPSenderContextCancellation(t, func(ctx context.Context, sender *BrevoSender) error {
		return sender.SendPasswordReset(ctx, "to@example.com", "token", "App", "https://example.com/reset", 15*time.Minute)
	})
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
	mediaType, params := requireMIMEType(t, message.Header.Get("Content-Type"))
	if mediaType != "multipart/mixed" {
		t.Fatalf("outer content type = %q", mediaType)
	}
	outer := multipart.NewReader(message.Body, params["boundary"])
	alternative := requireMIMEPart(t, outer)
	alternativeType, alternativeParams := requireMIMEType(t, alternative.Header.Get("Content-Type"))
	if alternativeType != "multipart/alternative" {
		t.Fatalf("alternative content type = %q", alternativeType)
	}
	inner := multipart.NewReader(alternative, alternativeParams["boundary"])
	assertMIMEBodyPart(t, requireMIMEPart(t, inner), "Plain body")
	assertMIMEBodyPart(t, requireMIMEPart(t, inner), "<p>HTML body</p>")
	assertMIMEAttachment(t, requireMIMEPart(t, outer), want)
}

func requireMIMEType(t *testing.T, value string) (string, map[string]string) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		t.Fatal(err)
	}
	return mediaType, params
}

func requireMIMEPart(t *testing.T, reader *multipart.Reader) *multipart.Part {
	t.Helper()
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	return part
}

func assertMIMEBodyPart(t *testing.T, part *multipart.Part, want string) {
	t.Helper()
	body, err := io.ReadAll(part)
	if err != nil || string(body) != want {
		t.Fatalf("MIME body = %q, %v; want %q", body, err, want)
	}
}

func assertMIMEAttachment(t *testing.T, part *multipart.Part, want []byte) {
	t.Helper()
	disposition, params := requireMIMEType(t, part.Header.Get("Content-Disposition"))
	if disposition != "attachment" || params["filename"] != "resume.pdf" {
		t.Fatalf("attachment disposition = %q", part.Header.Get("Content-Disposition"))
	}
	if part.Header.Get("Content-Transfer-Encoding") != "base64" {
		t.Fatalf("attachment transfer encoding = %q", part.Header.Get("Content-Transfer-Encoding"))
	}
	encoded, err := io.ReadAll(part)
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
