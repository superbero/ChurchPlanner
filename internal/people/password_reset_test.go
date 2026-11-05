package people

import (
	"strings"
	"testing"
)

func TestPasswordResetTokenIsOpaqueAndHashed(t *testing.T) {
	token, tokenHash, err := newPasswordResetToken()
	if err != nil {
		t.Fatalf("newPasswordResetToken() error = %v", err)
	}
	if len(token) != 43 || len(tokenHash) != 64 || strings.Contains(tokenHash, token) {
		t.Fatalf("unexpected token/hash lengths or token disclosure: token=%q hash=%q", token, tokenHash)
	}
	gotHash, err := hashPasswordResetToken(token)
	if err != nil {
		t.Fatalf("hashPasswordResetToken() error = %v", err)
	}
	if gotHash != tokenHash {
		t.Fatalf("hashPasswordResetToken() = %q, want %q", gotHash, tokenHash)
	}
	if _, err := hashPasswordResetToken("invalid"); err == nil {
		t.Fatal("hashPasswordResetToken() accepted an invalid token")
	}
}

func TestPasswordResetBaseURLRequiresAbsoluteHTTPURL(t *testing.T) {
	got, err := passwordResetBaseURL(" https://church.example/app/ ")
	if err != nil {
		t.Fatalf("passwordResetBaseURL() error = %v", err)
	}
	if got != "https://church.example/app" {
		t.Fatalf("passwordResetBaseURL() = %q", got)
	}
	for _, invalid := range []string{"", "/app", "ftp://church.example", "https://user@church.example"} {
		if _, err := passwordResetBaseURL(invalid); err == nil {
			t.Errorf("passwordResetBaseURL(%q) accepted invalid URL", invalid)
		}
	}
}
