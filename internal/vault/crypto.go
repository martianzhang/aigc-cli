// Package vault provides age-encrypted storage for sensitive documents.
// The age identity is the aigc-cli master secret, resolved through
// internal/secret (env override + system keyring).
package vault

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"
	"filippo.io/age/armor"

	"github.com/martianzhang/aigc-cli/internal/secret"
)

// generateIdentity creates a new age X25519 identity string.
func generateIdentity() (string, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", fmt.Errorf("generate identity: %w", err)
	}
	return identity.String(), nil
}

// EnsureIdentity returns the age identity, generating and storing a new one on
// first use when neither AIGC_CLI_MASTER_KEY nor the system keyring has it.
// created reports whether a new identity was generated.
func EnsureIdentity() (identity *age.X25519Identity, created bool, err error) {
	keyStr, created, err := secret.Ensure(generateIdentity)
	if err != nil {
		return nil, false, err
	}
	identity, err = age.ParseX25519Identity(keyStr)
	if err != nil {
		return nil, false, fmt.Errorf("parse identity: %w", err)
	}
	return identity, created, nil
}

// LoadIdentity loads the age identity from the env override or system keyring.
func LoadIdentity() (*age.X25519Identity, error) {
	keyStr, err := secret.Load()
	if err != nil {
		return nil, fmt.Errorf("key not found (set %s or run 'aigc-cli kb init' first): %w", secret.EnvVar, err)
	}

	identity, err := age.ParseX25519Identity(keyStr)
	if err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}

	return identity, nil
}

// IdentityExists reports whether a master identity is available.
func IdentityExists() bool {
	_, err := secret.Load()
	return err == nil
}

// Encrypt encrypts plaintext using the identity's recipient key.
// Returns ASCII-armored ciphertext.
func Encrypt(plaintext []byte) ([]byte, error) {
	identity, err := LoadIdentity()
	if err != nil {
		return nil, err
	}

	recipient := identity.Recipient()
	var buf bytes.Buffer
	armorWriter := armor.NewWriter(&buf)
	w, err := age.Encrypt(armorWriter, recipient)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("close encrypt: %w", err)
	}
	if err := armorWriter.Close(); err != nil {
		return nil, fmt.Errorf("close armor: %w", err)
	}

	return buf.Bytes(), nil
}

// Decrypt decrypts ASCII-armored ciphertext using the identity.
func Decrypt(ciphertext []byte) ([]byte, error) {
	identity, err := LoadIdentity()
	if err != nil {
		return nil, err
	}

	armorReader := armor.NewReader(bytes.NewReader(ciphertext))
	r, err := age.Decrypt(armorReader, identity)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	return plaintext, nil
}

// ImportIdentity stores an identity string (private key) as the master secret.
func ImportIdentity(keyStr string) error {
	return secret.Store(keyStr)
}
