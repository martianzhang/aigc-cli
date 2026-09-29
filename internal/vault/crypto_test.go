package vault

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/secret"
	"github.com/zalando/go-keyring"
)

// initMockSecret installs an in-memory keyring for the duration of the test.
func initMockSecret(t *testing.T) {
	t.Helper()
	t.Setenv(secret.EnvVar, "")
	t.Setenv(secret.DisableEnvVar, "")
	keyring.MockInit()
	secret.Reset()
	t.Cleanup(func() {
		keyring.MockInit()
		secret.Reset()
	})
}

func TestEnsureIdentityGeneratesOnFirstUse(t *testing.T) {
	initMockSecret(t)

	identity, created, err := EnsureIdentity()
	if err != nil {
		t.Fatalf("EnsureIdentity() error = %v", err)
	}
	if !created {
		t.Error("first EnsureIdentity() created = false, want true")
	}
	if !IdentityExists() {
		t.Error("IdentityExists() = false after EnsureIdentity()")
	}

	// A fresh view resolves the stored identity without regenerating.
	secret.Reset()
	second, created, err := EnsureIdentity()
	if err != nil {
		t.Fatalf("second EnsureIdentity() error = %v", err)
	}
	if created {
		t.Error("second EnsureIdentity() created = true, want false")
	}
	if second.String() != identity.String() {
		t.Error("stored identity changed between calls")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	initMockSecret(t)
	if _, _, err := EnsureIdentity(); err != nil {
		t.Fatalf("EnsureIdentity() error = %v", err)
	}

	plaintext := []byte("api_key: sk-secret")
	ciphertext, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if string(ciphertext) == string(plaintext) {
		t.Fatal("ciphertext leaked plaintext")
	}

	got, err := Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if string(got) != string(plaintext) {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

func TestImportIdentity(t *testing.T) {
	initMockSecret(t)

	original, _, err := EnsureIdentity()
	if err != nil {
		t.Fatalf("EnsureIdentity() error = %v", err)
	}
	keyStr := original.String()

	// Wipe and re-import through the public API.
	keyring.MockInit()
	secret.Reset()

	if err := ImportIdentity(keyStr); err != nil {
		t.Fatalf("ImportIdentity() error = %v", err)
	}
	loaded, err := LoadIdentity()
	if err != nil {
		t.Fatalf("LoadIdentity() error = %v", err)
	}
	if loaded.String() != keyStr {
		t.Error("LoadIdentity() returned a different identity than imported")
	}
}
