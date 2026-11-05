package mailer

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/smtp"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yourchurch/churchplanner/internal/platform/config"
)

type Mailer struct {
	cfg           config.Config
	db            *pgxpool.Pool
	encryptionKey []byte
}

var (
	ErrSMTPNotConfigured   = errors.New("SMTP is not configured")
	ErrEncryptionKeyNeeded = errors.New("APP_ENCRYPTION_KEY is required to store SMTP credentials")
)

type SMTPSettings struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
}

func New(cfg config.Config, db *pgxpool.Pool) *Mailer {
	key := sha256.Sum256([]byte(cfg.AppEncryptionKey))
	return &Mailer{cfg: cfg, db: db, encryptionKey: key[:]}
}

func (m *Mailer) CurrentSMTPSettings(ctx context.Context) (SMTPSettings, bool, error) {
	if m.db != nil {
		var settings SMTPSettings
		var encrypted []byte
		err := m.db.QueryRow(ctx, `
			SELECT host, port, username, password_encrypted, from_address
			FROM smtp_settings WHERE id = 1
		`).Scan(&settings.Host, &settings.Port, &settings.Username, &encrypted, &settings.FromAddress)
		switch {
		case err == nil:
			settings.Password, err = m.decrypt(encrypted)
			if err != nil {
				return SMTPSettings{}, false, err
			}
			return settings, true, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return SMTPSettings{}, false, err
		}
	}

	port, err := strconv.Atoi(m.cfg.SMTPPort)
	if err != nil {
		return SMTPSettings{}, false, fmt.Errorf("invalid SMTP_PORT: %w", err)
	}
	settings := SMTPSettings{
		Host:        m.cfg.SMTPHost,
		Port:        port,
		Username:    m.cfg.SMTPUser,
		Password:    m.cfg.SMTPPass,
		FromAddress: m.cfg.SMTPFrom,
	}
	return settings, settings.Host != "", nil
}

func (m *Mailer) SaveSMTPSettings(ctx context.Context, settings SMTPSettings) error {
	if m.db == nil {
		return errors.New("SMTP settings database is not available")
	}
	if settings.Password == "" {
		current, configured, err := m.CurrentSMTPSettings(ctx)
		if err != nil {
			return err
		}
		if !configured || current.Password == "" {
			return errors.New("SMTP password is required")
		}
		settings.Password = current.Password
	}
	encrypted, err := m.encrypt(settings.Password)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(ctx, `
		INSERT INTO smtp_settings (id, host, port, username, password_encrypted, from_address)
		VALUES (1, $1, $2, $3, $4, $5)
		ON CONFLICT (id) DO UPDATE SET
			host = EXCLUDED.host,
			port = EXCLUDED.port,
			username = EXCLUDED.username,
			password_encrypted = EXCLUDED.password_encrypted,
			from_address = EXCLUDED.from_address,
			updated_at = now()
	`, settings.Host, settings.Port, settings.Username, encrypted, settings.FromAddress)
	return err
}

func (m *Mailer) SendContext(ctx context.Context, to, subject, body string) error {
	settings, configured, err := m.CurrentSMTPSettings(ctx)
	if err != nil {
		return fmt.Errorf("load SMTP settings: %w", err)
	}
	if !configured || settings.Host == "" {
		return ErrSMTPNotConfigured
	}
	return sendSMTP(settings, to, subject, body)
}

func sendSMTP(settings SMTPSettings, to, subject, body string) error {
	msg := []byte(fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=UTF-8\r\n"+
		"\r\n"+
		"%s\r\n", settings.FromAddress, to, subject, body))

	auth := smtp.PlainAuth("", settings.Username, settings.Password, settings.Host)
	addr := fmt.Sprintf("%s:%d", settings.Host, settings.Port)

	err := smtp.SendMail(addr, auth, settings.FromAddress, []string{to}, msg)
	if err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}
	return nil
}

func (m *Mailer) SendTest(ctx context.Context, to string) error {
	return m.SendContext(ctx, to, "ChurchPlanner SMTP test", "This is a test email from ChurchPlanner. SMTP is configured correctly.")
}

func (m *Mailer) SendPasswordReset(ctx context.Context, to, name, link string) error {
	body := fmt.Sprintf(`Hi %s,

A password reset was requested for your ChurchPlanner account.

Use the one-time link below to choose a new password. It expires in 30 minutes.
If you did not expect this message, you can ignore it.

%s
`, name, link)
	return m.SendContext(ctx, to, "Reset your ChurchPlanner password", body)
}

// SendAssignmentNotification generates the email body for a new assignment.
func (m *Mailer) SendAssignmentNotification(ctx context.Context, toEmail, toName, position, planTitle, planDate, acceptURL string) error {
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

	return m.SendContext(ctx, toEmail, subject, body)
}

func (m *Mailer) encrypt(value string) ([]byte, error) {
	if m.cfg.AppEncryptionKey == "" {
		return nil, ErrEncryptionKeyNeeded
	}
	block, err := aes.NewCipher(m.encryptionKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(value), nil), nil
}

func (m *Mailer) decrypt(value []byte) (string, error) {
	if m.cfg.AppEncryptionKey == "" {
		return "", ErrEncryptionKeyNeeded
	}
	block, err := aes.NewCipher(m.encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(value) < gcm.NonceSize() {
		return "", errors.New("stored SMTP password is invalid")
	}
	nonce, ciphertext := value[:gcm.NonceSize()], value[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt SMTP password: %w", err)
	}
	return string(plaintext), nil
}
