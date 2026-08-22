package email

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/revieu-corp/revieu-core-api-go/apps/core/internal/config"
)

type SMTPClient struct {
	host     string
	port     int
	username string
	password string
	from     string
	useTLS   bool
}

func NewSMTPClient(cfg config.SMTPConfig) *SMTPClient {
	return &SMTPClient{
		host:     cfg.Host,
		port:     cfg.Port,
		username: cfg.Username,
		password: cfg.Password,
		from:     cfg.From,
		useTLS:   cfg.UseTLS,
	}
}

// SendEmail sends an email using SMTP
func (c *SMTPClient) SendEmail(to, subject, body string) error {
	return c.SendEmailHTML(to, subject, body, false)
}

// SendEmailHTML sends an email with optional HTML content
func (c *SMTPClient) SendEmailHTML(to, subject, body string, isHTML bool) error {
	addr := fmt.Sprintf("%s:%d", c.host, c.port)

	contentType := "text/plain"
	if isHTML {
		contentType = "text/html"
	}

	msg := strings.Builder{}
	msg.WriteString(fmt.Sprintf("From: %s\r\n", c.from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString(fmt.Sprintf("Content-Type: %s; charset=UTF-8\r\n", contentType))
	msg.WriteString("\r\n")
	msg.WriteString(body)

	auth := smtp.PlainAuth("", c.username, c.password, c.host)

	if c.useTLS {
		return c.sendWithTLS(addr, auth, to, msg.String())
	}
	return smtp.SendMail(addr, auth, c.from, []string{to}, []byte(msg.String()))
}

func (c *SMTPClient) sendWithTLS(addr string, auth smtp.Auth, to, msg string) error {
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: c.host})
	if err != nil {
		// Try STARTTLS instead
		return c.sendWithSTARTTLS(addr, auth, to, msg)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, c.host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}

	if err := client.Mail(c.from); err != nil {
		return fmt.Errorf("SMTP MAIL command failed: %w", err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT command failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA command failed: %w", err)
	}

	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("failed to write email body: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close email writer: %w", err)
	}

	return client.Quit()
}

func (c *SMTPClient) sendWithSTARTTLS(addr string, auth smtp.Auth, to, msg string) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return fmt.Errorf("failed to dial SMTP server: %w", err)
	}
	defer client.Close()

	if err := client.StartTLS(&tls.Config{ServerName: c.host}); err != nil {
		return fmt.Errorf("STARTTLS failed: %w", err)
	}

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth failed: %w", err)
	}

	if err := client.Mail(c.from); err != nil {
		return fmt.Errorf("SMTP MAIL command failed: %w", err)
	}

	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT command failed: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA command failed: %w", err)
	}

	if _, err := w.Write([]byte(msg)); err != nil {
		return fmt.Errorf("failed to write email body: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close email writer: %w", err)
	}

	return client.Quit()
}

// SendVerificationEmail sends a verification email to the user
func (c *SMTPClient) SendVerificationEmail(to, verifyURL string) error {
	subject := "Verify your email address"
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
</head>
<body>
    <h2>Welcome to RevieU!</h2>
    <p>Please click the link below to verify your email address:</p>
    <p><a href="%s">Verify Email</a></p>
    <p>Or copy and paste this URL into your browser:</p>
    <p>%s</p>
    <p>This link will expire in 24 hours.</p>
    <br>
    <p>If you did not create an account, please ignore this email.</p>
</body>
</html>
`, verifyURL, verifyURL)

	return c.SendEmailHTML(to, subject, body, true)
}

// SendPasswordResetEmail sends a single-use password reset link.
func (c *SMTPClient) SendPasswordResetEmail(to, resetURL string) error {
	subject := "Reset your RevieU password"
	body := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
</head>
<body>
    <h2>Reset your RevieU password</h2>
    <p>Click the link below to choose a new password:</p>
    <p><a href="%s">Reset password</a></p>
    <p>Or copy and paste this URL into your browser:</p>
    <p>%s</p>
    <p>This link will expire in one hour and can only be used once.</p>
    <br>
    <p>If you did not request a password reset, you can ignore this email.</p>
</body>
</html>
`, resetURL, resetURL)

	return c.SendEmailHTML(to, subject, body, true)
}
