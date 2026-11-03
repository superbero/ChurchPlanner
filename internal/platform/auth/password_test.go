package auth

import "testing"

func TestHashPasswordAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !CheckPassword(hash, "correct horse battery staple") {
		t.Fatal("CheckPassword() rejected the correct password")
	}
	if CheckPassword(hash, "incorrect password") {
		t.Fatal("CheckPassword() accepted an incorrect password")
	}
}

func TestCheckPasswordRejectsMalformedHash(t *testing.T) {
	if CheckPassword("not-a-bcrypt-hash", "password") {
		t.Fatal("CheckPassword() accepted a malformed hash")
	}
}

func TestHashPasswordRejectsOverlongPassword(t *testing.T) {
	if _, err := HashPassword(string(make([]byte, 73))); err == nil {
		t.Fatal("HashPassword() accepted a password longer than bcrypt's limit")
	}
}
