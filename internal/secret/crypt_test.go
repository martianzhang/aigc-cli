package secret

import (
	"errors"
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	reset(t)
	t.Setenv(EnvVar, "master-secret-1")

	plaintext := "sk-abcdefghijklmnop"
	encrypted, err := EncryptString(plaintext)
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	if !IsEncrypted(encrypted) {
		t.Errorf("EncryptString() = %q, want %s prefix", encrypted, EncryptedPrefix)
	}
	if strings.Contains(encrypted, plaintext) {
		t.Error("ciphertext leaks plaintext")
	}

	got, err := DecryptString(encrypted)
	if err != nil {
		t.Fatalf("DecryptString() error = %v", err)
	}
	if got != plaintext {
		t.Errorf("DecryptString() = %q, want %q", got, plaintext)
	}
}

func TestEncryptStringEmptyAndPlain(t *testing.T) {
	reset(t)
	t.Setenv(EnvVar, "master-secret-1")

	got, err := EncryptString("")
	if err != nil || got != "" {
		t.Fatalf("EncryptString(\"\") = (%q, %v), want (\"\", nil)", got, err)
	}

	plain := "not-encrypted"
	out, err := DecryptString(plain)
	if err != nil || out != plain {
		t.Fatalf("DecryptString(plain) = (%q, %v), want passthrough", out, err)
	}
	if IsEncrypted(plain) {
		t.Error("IsEncrypted(plain) = true")
	}
}

func TestEncryptStringRandomNonce(t *testing.T) {
	reset(t)
	t.Setenv(EnvVar, "master-secret-1")

	a, err := EncryptString("same")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	b, err := EncryptString("same")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}
	if a == b {
		t.Error("two encryptions of the same value must differ (random nonce)")
	}
}

func TestDecryptStringWrongKey(t *testing.T) {
	reset(t)
	t.Setenv(EnvVar, "key-A")
	encrypted, err := EncryptString("secret")
	if err != nil {
		t.Fatalf("EncryptString() error = %v", err)
	}

	// Swap the master secret and drop the process cache.
	Reset()
	t.Setenv(EnvVar, "key-B")
	if _, err := DecryptString(encrypted); err == nil {
		t.Fatal("DecryptString() with a different key = nil error, want failure")
	}
}

func TestDecryptStringCorrupt(t *testing.T) {
	reset(t)
	t.Setenv(EnvVar, "master-secret-1")

	for _, value := range []string{
		EncryptedPrefix + "not-base64!!",
		EncryptedPrefix + "AAAA", // decodes but too short
	} {
		if _, err := DecryptString(value); err == nil {
			t.Errorf("DecryptString(%q) = nil error, want failure", value)
		} else if errors.Is(err, ErrNotFound) {
			t.Errorf("DecryptString(%q) error = %v, want non-notfound", value, err)
		}
	}
}
