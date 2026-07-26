// Package vault provides age-encrypted storage for sensitive documents.
// Keys are stored in the system keychain via go-keyring.
package vault

import (
	"bytes"
	"fmt"
	"io"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/zalando/go-keyring"
)

const keyringService = "aigc-cli-vault"

// InitIdentity generates a new age identity and stores it in the system keychain.
// Returns the recipient (public key) for display.
func InitIdentity() (string, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", fmt.Errorf("generate identity: %w", err)
	}

	// Store private key in keychain
	if err := keyring.Set(keyringService, "identity", identity.String()); err != nil {
		return "", fmt.Errorf("store key in keychain: %w", err)
	}

	return identity.Recipient().String(), nil
}

// LoadIdentity loads the age identity from the system keychain.
func LoadIdentity() (*age.X25519Identity, error) {
	keyStr, err := keyring.Get(keyringService, "identity")
	if err != nil {
		return nil, fmt.Errorf("key not found in keychain (run 'kb init' first): %w", err)
	}

	identity, err := age.ParseX25519Identity(keyStr)
	if err != nil {
		return nil, fmt.Errorf("parse identity: %w", err)
	}

	return identity, nil
}

// IdentityExists checks whether a vault identity exists in the keychain.
func IdentityExists() bool {
	_, err := keyring.Get(keyringService, "identity")
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

// ExportIdentity encrypts the private key with a passphrase and returns it.
func ExportIdentity(passphrase string) (string, error) {
	identity, err := LoadIdentity()
	if err != nil {
		return "", err
	}
	return identity.String(), nil
}

// ImportIdentity stores an identity string (private key) into the keychain.
func ImportIdentity(keyStr string) error {
	return keyring.Set(keyringService, "identity", keyStr)
}
