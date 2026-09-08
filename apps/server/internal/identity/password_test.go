package identity

import (
	"errors"
	"strings"
	"testing"
)

var fastPasswordParams = passwordParams{
	time:    1,
	memory:  8 * 1024,
	threads: 1,
	keyLen:  32,
}

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := hashPasswordWithParams("correct horse battery staple", fastPasswordParams)
	if err != nil {
		t.Fatalf("hashPasswordWithParams() error = %v", err)
	}
	if strings.Contains(hash, "correct horse") {
		t.Fatal("password hash contains plaintext password")
	}

	valid, err := verifyPassword(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("verifyPassword() error = %v", err)
	}
	if !valid {
		t.Fatal("verifyPassword() = false, want true")
	}

	valid, err = verifyPassword(hash, "wrong password")
	if err != nil {
		t.Fatalf("verifyPassword(wrong) error = %v", err)
	}
	if valid {
		t.Fatal("verifyPassword(wrong) = true, want false")
	}
}

func TestPasswordHashesUseUniqueSalts(t *testing.T) {
	first, err := hashPasswordWithParams("same password", fastPasswordParams)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := hashPasswordWithParams("same password", fastPasswordParams)
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first == second {
		t.Fatal("two password hashes are equal, want unique salts")
	}
}

func TestPasswordRejectsMalformedHash(t *testing.T) {
	tests := []string{
		"not-a-password-hash",
		"$argon2id$v=19junk$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2FsdA$YWJjZGVmZ2hpamtsbW5vcA",
		"$argon2id$v=19$m=8192,t=1,p=1junk$c2FsdHNhbHRzYWx0c2FsdA$YWJjZGVmZ2hpamtsbW5vcA",
	}

	for _, encoded := range tests {
		valid, err := verifyPassword(encoded, "password")
		if valid || !errors.Is(err, errInvalidPasswordHash) {
			t.Fatalf("verifyPassword(%q) = %v, %v; want false, errInvalidPasswordHash", encoded, valid, err)
		}
	}
}
