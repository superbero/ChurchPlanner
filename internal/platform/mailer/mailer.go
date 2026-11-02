package mailer

import (
	"fmt"
	"log/slog"
	"net/smtp"

	"github.com/yourchurch/churchplanner/internal/platform/config"
)

type Mailer struct {
	cfg config.Config
}

func New(cfg config.Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// Send sends a plain text email. If SMTP_HOST is empty, it logs to stdout.
func (m *Mailer) Send(to, subject, body string) error {
	if m.cfg.SMTPHost == "" {
		slog.Info("MOCK EMAIL", "to", to, "subject", subject, "body", body)
		return nil
	}

	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"\r\n"+
		"%s\r\n", m.cfg.SMTPFrom, to, subject, body))

	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%s", m.cfg.SMTPHost, m.cfg.SMTPPort)

	err := smtp.SendMail(addr, auth, m.cfg.SMTPFrom, []string{to}, msg)
	if err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}
	return nil
}

// SendAssignmentNotification generates the email body for a new assignment.
func (m *Mailer) SendAssignmentNotification(toEmail, toName, position, planTitle, planDate, acceptURL string) error {
	subject := "You are scheduled for " + planTitle
	body := fmt.Sprintf(`Hi %s,

You have been scheduled for:
Role: %s
Service: %s
Date: %s

Please click the link below to confirm or decline this assignment:
%s

If you cannot make it, please decline so we can find a replacement.

- The Scheduling Team
`, toName, position, planTitle, planDate, acceptURL)

	return m.Send(toEmail, subject, body)
}
