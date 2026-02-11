package mailer

import (
	"bytes"
	"fmt"
	"html"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultSMTPPort = 587

// BrevoSender sends OTP emails using Brevo SMTP (compatible with net/smtp).
type BrevoSender struct {
	Host         string
	Port         int
	User         string
	Pass         string
	From         string
	Subject      string
	ResetSubject string
	TTL          time.Duration
	AppName      string
}

func NewBrevoSenderFromEnv() (*BrevoSender, error) {
	port := defaultSMTPPort
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	host := os.Getenv("SMTP_HOST")
	user := os.Getenv("SMTP_USER")
	pass := os.Getenv("SMTP_PASS")
	from := os.Getenv("SMTP_FROM")

	if host == "" || pass == "" || from == "" {
		return nil, fmt.Errorf("smtp credentials not configured")
	}
	if user == "" {
		user = "apikey"
	}

	subject := os.Getenv("SMTP_SUBJECT")
	if subject == "" {
		subject = "Your Registration OTP"
	}

	resetSubject := os.Getenv("RESET_SUBJECT")
	if resetSubject == "" {
		resetSubject = "Reset Your Password"
	}

	appName := os.Getenv("OTP_APP_NAME")
	if appName == "" {
		appName = "Account Verification"
	}

	ttl := parseDurationEnv([]string{"OTP_TTL"}, 5*time.Minute)

	return &BrevoSender{
		Host:         host,
		Port:         port,
		User:         user,
		Pass:         pass,
		From:         from,
		Subject:      subject,
		ResetSubject: resetSubject,
		TTL:          ttl,
		AppName:      appName,
	}, nil
}

func (s *BrevoSender) SendOTP(to, code, appName string) error {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	auth := smtp.PlainAuth("", s.User, s.Pass, s.Host)

	if strings.TrimSpace(appName) == "" {
		appName = s.AppName
	}
	msg := buildOTPMessage(s.From, to, s.Subject, appName, code, s.TTL)

	return smtp.SendMail(addr, auth, extractEmail(s.From), []string{to}, msg)
}

func buildOTPMessage(from, to, subject, appName, code string, ttl time.Duration) []byte {
	minutes := int(ttl.Minutes())
	if minutes <= 0 {
		minutes = 5
	}

	safeAppName := html.EscapeString(appName)

	textBody := fmt.Sprintf(
		"Kode verifikasi pendaftaran kamu: %s\nKode ini akan kadaluarsa dalam %d menit.\nJika kamu tidak merasa mendaftar, abaikan email ini.\n",
		code,
		minutes,
	)
	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="id">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Kode Verifikasi - %s</title>
</head>
<body style="margin: 0; padding: 0; font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: #f8f9fa;">
  <div style="padding: 40px 20px; min-height: 100%%;">
    <div style="max-width: 480px; margin: 0 auto; background-color: #ffffff; border-radius: 12px; overflow: hidden; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.05);">
      
      <div style="background-color: #1a1a2e; padding: 32px 24px; text-align: center;">
        <h1 style="color: #ffffff; font-size: 24px; font-weight: 600; margin: 0; letter-spacing: 0.5px;">
          %s
        </h1>
      </div>

      <div style="padding: 40px 32px;">
        <h2 style="color: #1a1a2e; font-size: 20px; font-weight: 600; margin: 0 0 16px 0;">
          Verifikasi Akun Anda
        </h2>

        <p style="color: #4a5568; font-size: 15px; line-height: 1.6; margin: 0 0 24px 0;">
          Halo <strong>Pengguna</strong>,
        </p>

        <p style="color: #4a5568; font-size: 15px; line-height: 1.6; margin: 0 0 32px 0;">
          Gunakan kode verifikasi berikut untuk menyelesaikan pendaftaran akun Anda:
        </p>

        <div style="background-color: #f7f7f9; border-radius: 8px; padding: 24px; text-align: center; margin-bottom: 32px; border: 1px dashed #e2e8f0;">
          <p style="color: #718096; font-size: 12px; text-transform: uppercase; letter-spacing: 1px; margin: 0 0 12px 0;">
            Kode Verifikasi
          </p>
          <p style="color: #1a1a2e; font-size: 36px; font-weight: 700; letter-spacing: 8px; margin: 0; font-family: 'Courier New', monospace;">
            %s
          </p>
        </div>

        <div style="background-color: #fff8e6; border-radius: 6px; padding: 12px 16px; margin-bottom: 24px; border-left: 3px solid #f6ad55;">
          <p style="color: #744210; font-size: 13px; margin: 0; line-height: 1.5;">
            ⏱️ Kode ini akan kadaluarsa dalam <strong>%d menit</strong>
          </p>
        </div>

        <div style="background-color: #f0f9ff; border-radius: 6px; padding: 12px 16px; border-left: 3px solid #63b3ed;">
          <p style="color: #2b6cb0; font-size: 13px; margin: 0; line-height: 1.5;">
            🔒 Jangan bagikan kode ini kepada siapapun termasuk pihak yang mengaku dari %s.
          </p>
        </div>
      </div>

      <div style="background-color: #f8f9fa; padding: 24px 32px; border-top: 1px solid #e2e8f0;">
        <p style="color: #a0aec0; font-size: 12px; text-align: center; margin: 0 0 8px 0; line-height: 1.5;">
          Jika Anda tidak merasa mendaftar di %s, abaikan email ini.
        </p>
        <p style="color: #a0aec0; font-size: 12px; text-align: center; margin: 0;">
          © %d %s. All rights reserved.
        </p>
      </div>

    </div>
  </div>
</body>
</html>`,
		safeAppName,
		safeAppName,
		code,
		minutes,
		safeAppName,
		safeAppName,
		time.Now().Year(),
		safeAppName,
	)

	boundary := "otp-boundary"

	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(textBody + "\r\n")
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	buf.WriteString(htmlBody + "\r\n")
	buf.WriteString("--" + boundary + "--")

	return buf.Bytes()
}

func (s *BrevoSender) SendPasswordReset(to, token, appName, resetURL string, ttl time.Duration) error {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	auth := smtp.PlainAuth("", s.User, s.Pass, s.Host)

	if strings.TrimSpace(appName) == "" {
		appName = s.AppName
	}
	subject := s.ResetSubject
	if subject == "" {
		subject = "Reset Your Password"
	}

	msg := buildPasswordResetMessage(s.From, to, subject, appName, token, resetURL, ttl)
	return smtp.SendMail(addr, auth, extractEmail(s.From), []string{to}, msg)
}

func buildPasswordResetMessage(from, to, subject, appName, token, resetURL string, ttl time.Duration) []byte {
	minutes := int(ttl.Minutes())
	if minutes <= 0 {
		minutes = 15
	}

	safeAppName := html.EscapeString(appName)
	safeURL := html.EscapeString(resetURL)

	textBody := fmt.Sprintf(
		"Kami menerima permintaan reset password untuk akun %s.\\n",
		safeAppName,
	)
	if resetURL != "" {
		textBody += fmt.Sprintf("Link reset: %s\\n", resetURL)
	} else {
		textBody += fmt.Sprintf("Token reset: %s\\n", token)
	}
	expiryLabel := "Token ini"
	if resetURL != "" {
		expiryLabel = "Link ini"
	}
	textBody += fmt.Sprintf("%s kadaluarsa dalam %d menit.\\nJika kamu tidak meminta reset, abaikan email ini.\\n", expiryLabel, minutes)

	linkSection := ""
	linkText := ""
	tokenSection := ""
	if resetURL != "" {
		linkSection = fmt.Sprintf(`<div style="margin: 18px 0 16px 0; text-align:center;">
  <a href="%s" style="background:#2563eb;color:#ffffff;text-decoration:none;padding:12px 18px;border-radius:10px;font-weight:600;display:inline-block;">
    Reset Password
  </a>
</div>`, safeURL)
		linkText = fmt.Sprintf(`<p style="color:#64748b;font-size:12px;line-height:1.5;margin:0 0 18px 0;">
Jika tombol di atas tidak berfungsi, salin link berikut ke browser Anda:
<br><a href="%s" style="color:#2563eb;word-break:break-all;">%s</a>
</p>`, safeURL, safeURL)
	} else {
		tokenSection = fmt.Sprintf(`<div style="background-color: #f7f7f9; border-radius: 8px; padding: 18px; text-align: center; margin-bottom: 20px; border: 1px dashed #e2e8f0;">
          <p style="color: #718096; font-size: 12px; text-transform: uppercase; letter-spacing: 1px; margin: 0 0 10px 0;">
            Token Reset
          </p>
          <p style="color: #1a1a2e; font-size: 20px; font-weight: 700; letter-spacing: 2px; margin: 0; font-family: 'Courier New', monospace;">
            %s
          </p>
        </div>`, token)
	}

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="id">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Reset Password - %s</title>
</head>
<body style="margin: 0; padding: 0; font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; background-color: #f8f9fa;">
  <div style="padding: 40px 20px; min-height: 100%%;">
    <div style="max-width: 480px; margin: 0 auto; background-color: #ffffff; border-radius: 12px; overflow: hidden; box-shadow: 0 4px 6px rgba(0, 0, 0, 0.05);">
      <div style="background-color: #1a1a2e; padding: 32px 24px; text-align: center;">
        <h1 style="color: #ffffff; font-size: 24px; font-weight: 600; margin: 0; letter-spacing: 0.5px;">
          %s
        </h1>
      </div>
      <div style="padding: 36px 32px;">
        <h2 style="color: #1a1a2e; font-size: 20px; font-weight: 600; margin: 0 0 12px 0;">
          Reset Password
        </h2>
        <p style="color: #4a5568; font-size: 14px; line-height: 1.6; margin: 0 0 16px 0;">
          Kami menerima permintaan reset password untuk akun Anda.
        </p>
        %s
        %s
        %s
        <div style="background-color: #fff8e6; border-radius: 6px; padding: 12px 16px; margin-bottom: 18px; border-left: 3px solid #f6ad55;">
          <p style="color: #744210; font-size: 13px; margin: 0; line-height: 1.5;">
            ⏱️ %s kadaluarsa dalam <strong>%d menit</strong>
          </p>
        </div>
        <div style="background-color: #f0f9ff; border-radius: 6px; padding: 12px 16px; border-left: 3px solid #63b3ed;">
          <p style="color: #2b6cb0; font-size: 13px; margin: 0; line-height: 1.5;">
            🔒 Jika Anda tidak meminta reset password, abaikan email ini.
          </p>
        </div>
      </div>
      <div style="background-color: #f8f9fa; padding: 24px 32px; border-top: 1px solid #e2e8f0;">
        <p style="color: #a0aec0; font-size: 12px; text-align: center; margin: 0;">
          © %d %s. All rights reserved.
        </p>
      </div>
    </div>
  </div>
</body>
</html>`,
		safeAppName,
		safeAppName,
		linkSection,
		linkText,
		tokenSection,
		expiryLabel,
		minutes,
		time.Now().Year(),
		safeAppName,
	)

	boundary := "reset-boundary"

	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(textBody + "\r\n")
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	buf.WriteString(htmlBody + "\r\n")
	buf.WriteString("--" + boundary + "--")

	return buf.Bytes()
}

func (s *BrevoSender) SendEmail(payload EmailPayload) error {
	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
	auth := smtp.PlainAuth("", s.User, s.Pass, s.Host)

	appName := strings.TrimSpace(payload.AppName)
	if appName == "" {
		appName = s.AppName
	}

	subject := strings.TrimSpace(payload.Subject)
	if subject == "" {
		subject = "Information"
	}

	emailType := strings.ToUpper(strings.TrimSpace(payload.Type))
	if emailType == "" {
		emailType = "INFO"
	}
	finalSubject := fmt.Sprintf("[%s] %s", emailType, subject)

	textBody := strings.TrimSpace(payload.TextBody)
	htmlBody := strings.TrimSpace(payload.HTMLBody)
	if textBody == "" && htmlBody != "" {
		textBody = "Please open this email in HTML mode."
	}

	for _, recipient := range payload.To {
		to := strings.TrimSpace(recipient)
		if to == "" {
			continue
		}
		msg := buildGeneralMessage(s.From, to, payload.ReplyTo, finalSubject, appName, textBody, htmlBody, payload.IdempotencyKey)
		if err := smtp.SendMail(addr, auth, extractEmail(s.From), []string{to}, msg); err != nil {
			return fmt.Errorf("send to %s: %w", to, err)
		}
	}

	return nil
}

func buildGeneralMessage(from, to, replyTo, subject, appName, textBody, htmlBody, idempotencyKey string) []byte {
	if textBody == "" {
		textBody = "No text content provided."
	}
	if htmlBody == "" {
		htmlBody = "<p>" + html.EscapeString(textBody) + "</p>"
	}

	boundary := "general-boundary"

	var buf bytes.Buffer
	buf.WriteString("From: " + from + "\r\n")
	buf.WriteString("To: " + to + "\r\n")
	if strings.TrimSpace(replyTo) != "" {
		buf.WriteString("Reply-To: " + replyTo + "\r\n")
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		buf.WriteString("X-Idempotency-Key: " + idempotencyKey + "\r\n")
	}
	if strings.TrimSpace(appName) != "" {
		buf.WriteString("X-App-Name: " + appName + "\r\n")
	}
	buf.WriteString("Subject: " + subject + "\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")

	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	buf.WriteString(textBody + "\r\n")
	buf.WriteString("--" + boundary + "\r\n")
	buf.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
	buf.WriteString(htmlBody + "\r\n")
	buf.WriteString("--" + boundary + "--")

	return buf.Bytes()
}

func extractEmail(from string) string {
	start := strings.IndexByte(from, '<')
	end := strings.IndexByte(from, '>')
	if start >= 0 && end > start {
		return from[start+1 : end]
	}
	return from
}

func parseDurationEnv(keys []string, fallback time.Duration) time.Duration {
	for _, key := range keys {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			continue
		}
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
		if i, err := strconv.Atoi(value); err == nil {
			return time.Duration(i) * time.Second
		}
	}
	return fallback
}
