package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/secret"
)

// withMasterKey pins the master secret to a fixed value for the test.
func withMasterKey(t *testing.T, key string) {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv(secret.EnvVar, key)
	secret.Reset()
	t.Cleanup(func() { secret.Reset() })
}

const secretsFixture = `# my config
api_key: sk-global-plain

providers:
  openai:
    api_key: sk-provider-plain
    base_url: https://api.openai.com/v1

web_search:
  doubao:
    api_key: sk-search-plain
`

func TestEncryptSecretsInFile(t *testing.T) {
	withMasterKey(t, "master-key-1")
	path := writeFixture(t, "config.yaml", secretsFixture)

	count, err := EncryptSecretsInFile(path)
	if err != nil {
		t.Fatalf("EncryptSecretsInFile() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("encrypted %d secrets, want 3", count)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "sk-global-plain") || strings.Contains(text, "sk-provider-plain") || strings.Contains(text, "sk-search-plain") {
		t.Errorf("plaintext secret survived encryption:\n%s", text)
	}
	if strings.Count(text, secret.EncryptedPrefix) != 3 {
		t.Errorf("want 3 encrypted values:\n%s", text)
	}
	if !strings.Contains(text, "# my config") {
		t.Errorf("comment lost:\n%s", text)
	}
	if !strings.Contains(text, "base_url: https://api.openai.com/v1") {
		t.Errorf("non-secret value changed:\n%s", text)
	}
	// No plaintext backup may be written.
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("plaintext backup written: stat err = %v", err)
	}

	// Loading decrypts everything back to plaintext.
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.APIKey != "sk-global-plain" {
		t.Errorf("APIKey = %q", cfg.APIKey)
	}
	if cfg.Providers["openai"] == nil || cfg.Providers["openai"].APIKey != "sk-provider-plain" {
		t.Errorf("provider api_key not decrypted: %+v", cfg.Providers["openai"])
	}
	if cfg.WebSearch["doubao"] == nil || cfg.WebSearch["doubao"].APIKey != "sk-search-plain" {
		t.Errorf("web_search api_key not decrypted: %+v", cfg.WebSearch["doubao"])
	}
}

func TestEncryptSecretsInFileIdempotent(t *testing.T) {
	withMasterKey(t, "master-key-1")
	path := writeFixture(t, "config.yaml", secretsFixture)

	if _, err := EncryptSecretsInFile(path); err != nil {
		t.Fatalf("first encrypt: %v", err)
	}
	first, _ := os.ReadFile(path)

	count, err := EncryptSecretsInFile(path)
	if err != nil {
		t.Fatalf("second encrypt: %v", err)
	}
	if count != 0 {
		t.Errorf("second encrypt count = %d, want 0", count)
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Error("idempotent encrypt rewrote the file")
	}
}

func TestEncryptSecretsInFileMissing(t *testing.T) {
	withMasterKey(t, "master-key-1")
	count, err := EncryptSecretsInFile(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || count != 0 {
		t.Fatalf("EncryptSecretsInFile(missing) = (%d, %v), want (0, nil)", count, err)
	}
}

func TestLoadDecryptError(t *testing.T) {
	withMasterKey(t, "key-A")
	encrypted, err := secret.EncryptString("sk-secret")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	path := writeFixture(t, "config.yaml", "api_key: "+encrypted+"\n")

	// Switch to a different master secret: decryption must fail loudly.
	t.Setenv(secret.EnvVar, "key-B")
	secret.Reset()

	_, err = Load(path)
	if !errors.Is(err, ErrDecrypt) {
		t.Fatalf("Load() error = %v, want ErrDecrypt", err)
	}
	if !strings.Contains(err.Error(), secret.EnvVar) {
		t.Errorf("error should suggest %s: %v", secret.EnvVar, err)
	}
}
