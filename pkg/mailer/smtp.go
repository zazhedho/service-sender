package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/smtp"
	"time"
)

func sendMailContext(parent context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte) (err error) {
	return sendMailContextWithTLSConfig(parent, addr, auth, from, to, msg, nil)
}

func sendMailContextWithTLSConfig(parent context.Context, addr string, auth smtp.Auth, from string, to []string, msg []byte, tlsConfig *tls.Config) (err error) {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	defer func() {
		if err == nil {
			return
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			return
		}
		var networkError net.Error
		if errors.As(err, &networkError) && networkError.Timeout() && !time.Now().Before(deadline) {
			err = context.DeadlineExceeded
		}
	}()
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("SMTP server does not support required STARTTLS")
	}
	if tlsConfig == nil {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		tlsConfig = tlsConfig.Clone()
		if tlsConfig.MinVersion < tls.VersionTLS12 {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
	}
	tlsConfig.ServerName = host
	if err = client.StartTLS(tlsConfig); err != nil {
		return err
	}
	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err = client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write(msg); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
