package mailer

import (
	"bytes"
	"errors"
	"testing"

	"github.com/yourchurch/churchplanner/internal/platform/config"
)

func TestSMTPPasswordEncryption(t *testing.T) {
	m := New(config.Config{AppEncryptionKey: "a long random deployment secret"}, nil)
	encrypted, err := m.encrypt("gmail-app-password")
	if err != nil {
		t.Fatalf("encrypt() error = %v", err)
	}
	if bytes.Contains(encrypted, []byte("gmail-app-password")) {
		t.Fatal("encrypted SMTP password contains plaintext")
	}
	plain, err := m.decrypt(encrypted)
	if err != nil {
		t.Fatalf("decrypt() error = %v", err)
	}
	if plain != "gmail-app-password" {
		t.Fatalf("decrypted password = %q", plain)
	}

	wrongKey := New(config.Config{AppEncryptionKey: "a different deployment secret"}, nil)
	if _, err := wrongKey.decrypt(encrypted); err == nil {
		t.Fatal("decrypt() succeeded with a different key")
	}
}

func TestSMTPPasswordEncryptionRequiresKey(t *testing.T) {
	m := New(config.Config{}, nil)
	if _, err := m.encrypt("password"); !errors.Is(err, ErrEncryptionKeyNeeded) {
		t.Fatalf("encrypt() error = %v, want %v", err, ErrEncryptionKeyNeeded)
	}
}
