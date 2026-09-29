package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"strings"

	"encoding/base64"
)

// EncryptedPrefix marks a config value encrypted with EncryptString. The
// version segment allows the scheme to change without guessing.
const EncryptedPrefix = "enc:v1:"

// derivePurpose is the HKDF domain for config-value encryption. It is distinct
// from other DeriveKey callers so keys never overlap.
const derivePurpose = "config-v1"

// IsEncrypted reports whether value carries the encrypted-value marker.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, EncryptedPrefix)
}

// EncryptString encrypts a config value with AES-256-GCM under a key derived
// from the master secret. Empty values pass through unchanged so an unset key
// stays unset.
func EncryptString(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	gcm, err := configGCM()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return EncryptedPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// DecryptString reverses EncryptString. Values without the marker pass through
// unchanged, so plaintext configs keep working.
func DecryptString(value string) (string, error) {
	if !IsEncrypted(value) {
		return value, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, EncryptedPrefix))
	if err != nil {
		return "", fmt.Errorf("decode encrypted value: %w", err)
	}
	gcm, err := configGCM()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted value is too short")
	}
	nonce, ciphertext := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt value: %w", err)
	}
	return string(plaintext), nil
}

// configGCM builds the AES-256-GCM AEAD for config values.
func configGCM() (cipher.AEAD, error) {
	key, err := DeriveKey(derivePurpose)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init gcm: %w", err)
	}
	return gcm, nil
}
