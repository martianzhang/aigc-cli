package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/secret"
)

func TestConfigSetPreservesCommentsAndOrder(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	out, err := runConfigCmd(t, path, "set", "defaults.image.model", "gpt-image-2")
	if err != nil {
		t.Fatalf("config set error = %v", err)
	}
	if !strings.Contains(out, "set defaults.image.model = gpt-image-2") {
		t.Errorf("set output = %q", out)
	}

	got := readConfigFile(t, path)
	for _, comment := range []string{
		"# top-level comment",
		"# image section comment",
		"# model comment",
	} {
		if !strings.Contains(got, comment) {
			t.Errorf("comment %q did not survive:\n%s", comment, got)
		}
	}
	if !strings.Contains(got, "model: gpt-image-2") {
		t.Errorf("new value missing:\n%s", got)
	}
	providerAt := strings.Index(got, "provider: agnes")
	modelAt := strings.Index(got, "model: gpt-image-2")
	sizeAt := strings.Index(got, "size: '1024x768'")
	if providerAt < 0 || modelAt < 0 || sizeAt < 0 || providerAt > modelAt || modelAt > sizeAt {
		t.Errorf("key order changed:\n%s", got)
	}
}

func TestConfigSetKeepsIntType(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	if _, err := runConfigCmd(t, path, "set", "defaults.chat.max_iterations", "20"); err != nil {
		t.Fatalf("config set error = %v", err)
	}
	got := readConfigFile(t, path)
	if !strings.Contains(got, "max_iterations: 20") {
		t.Errorf("int value was not written as an int:\n%s", got)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load error = %v", err)
	}
	if cfg.Defaults == nil || cfg.Defaults.Chat == nil || cfg.Defaults.Chat.MaxIterations != 20 {
		t.Errorf("re-read MaxIterations = %+v, want 20", cfg.Defaults.Chat)
	}
}

func TestConfigSetCreatesLeafInExistingParent(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	if _, err := runConfigCmd(t, path, "set", "defaults.chat.max_tokens", "4096"); err != nil {
		t.Fatalf("config set int leaf error = %v", err)
	}
	if _, err := runConfigCmd(t, path, "set", "defaults.chat.model", "deepseek-v4-flash"); err != nil {
		t.Fatalf("config set string leaf error = %v", err)
	}
	got := readConfigFile(t, path)
	if !strings.Contains(got, "max_tokens: 4096") {
		t.Errorf("new int leaf is not an int:\n%s", got)
	}
	if !strings.Contains(got, "model: deepseek-v4-flash") {
		t.Errorf("new string leaf missing:\n%s", got)
	}
}

func TestConfigSetUnknownParent(t *testing.T) {
	path := writeCmdConfig(t, "defaults:\n  image:\n    model: agnes-image-2.5-flash\n")

	_, err := runConfigCmd(t, path, "set", "defaults.chat.allow_tool_override", "true")
	if err == nil {
		t.Fatal("config set missing parent = nil error")
	}
	want := "set defaults.chat.allow_tool_override: section not found"
	if err.Error() != want {
		t.Errorf("config set error = %q, want %q", err.Error(), want)
	}
	if strings.Contains(readConfigFile(t, path), "chat") {
		t.Error("missing section was auto-created")
	}
}

func TestConfigSetAPIKeyRequiresForce(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)
	before := readConfigFile(t, path)
	// Sensitive values are encrypted before they reach the file, so the test
	// needs a master key available.
	t.Setenv(secret.EnvVar, "test-master-key")
	secret.Reset()
	t.Cleanup(secret.Reset)

	_, err := runConfigCmd(t, path, "set", "api_key", "sk-replaced")
	if err == nil {
		t.Fatal("config set api_key without --force = nil error")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("config set api_key error = %q, want --force hint", err)
	}
	if got := readConfigFile(t, path); got != before {
		t.Error("config file changed despite the refusal")
	}

	out, err := runConfigCmd(t, path, "set", "api_key", "sk-replaced", "--force")
	if err != nil {
		t.Fatalf("config set api_key --force error = %v", err)
	}
	if strings.Contains(out, "sk-replaced") {
		t.Errorf("set echoed the new api_key in full: %q", out)
	}
	got := readConfigFile(t, path)
	if strings.Contains(got, "sk-replaced") {
		t.Errorf("api_key written in plaintext:\n%s", got)
	}
	if !strings.Contains(got, "api_key: "+secret.EncryptedPrefix) {
		t.Errorf("api_key not stored encrypted:\n%s", got)
	}
}

func TestPrintConfigEncryptsPlaintextSecrets(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)
	t.Setenv(secret.EnvVar, "test-master-key")
	secret.Reset()
	t.Cleanup(secret.Reset)

	prev := shared.CfgFile
	shared.CfgFile = path
	t.Cleanup(func() { shared.CfgFile = prev })

	runPrintConfig(newConfigCmd())

	got := readConfigFile(t, path)
	if strings.Contains(got, "sk-1234567890abcd") || strings.Contains(got, "sk-provider-secret-1234") {
		t.Errorf("plaintext secrets survived --print-config:\n%s", got)
	}
	if strings.Count(got, secret.EncryptedPrefix) != 2 {
		t.Errorf("want 2 encrypted secrets after --print-config:\n%s", got)
	}
}

func TestEncryptPlaintextConfigSecrets(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)
	t.Setenv(secret.EnvVar, "test-master-key")
	secret.Reset()
	t.Cleanup(secret.Reset)

	prev := shared.CfgFile
	shared.CfgFile = path
	t.Cleanup(func() { shared.CfgFile = prev })

	encryptPlaintextConfigSecrets()

	got := readConfigFile(t, path)
	if strings.Contains(got, "sk-1234567890abcd") || strings.Contains(got, "sk-provider-secret-1234") {
		t.Errorf("plaintext secrets survived startup encryption:\n%s", got)
	}
	if strings.Count(got, secret.EncryptedPrefix) != 2 {
		t.Errorf("want 2 encrypted secrets:\n%s", got)
	}
}

func TestConfigSetProviderBaseURLRequiresForce(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	_, err := runConfigCmd(t, path, "set", "providers.demo.base_url", "https://evil.example.com")
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("config set provider base_url error = %v, want --force hint", err)
	}
	if strings.Contains(readConfigFile(t, path), "evil.example.com") {
		t.Error("provider base_url changed despite the refusal")
	}
}

func TestConfigSetAtomicBackup(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)
	original := readConfigFile(t, path)

	if _, err := runConfigCmd(t, path, "set", "defaults.image.size", "512x512"); err != nil {
		t.Fatalf("config set error = %v", err)
	}
	if backup := readConfigFile(t, path+".bak"); backup != original {
		t.Error("backup does not match the original file")
	}
	if got := readConfigFile(t, path); !strings.Contains(got, "512x512") {
		t.Errorf("target not updated:\n%s", got)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatalf("ReadDir error = %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp.") {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}
