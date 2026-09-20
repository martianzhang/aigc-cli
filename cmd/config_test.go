package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const configTestFixture = `# top-level comment
api_key: sk-1234567890abcd
base_url: https://api.example.com/v1

defaults:
  # image section comment
  image:
    provider: agnes
    model: agnes-image-2.5-flash  # model comment
    size: '1024x768'
  chat:
    max_iterations: 5
providers:
  demo:
    api_key: sk-provider-secret-1234
    base_url: https://relay.example.com/v1
`

// writeCmdConfig writes a config fixture into a temp dir.
func writeCmdConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

// runConfigCmd executes the config command tree with shared.CfgFile pointed at
// cfgPath (which may be empty to exercise the default path logic).
func runConfigCmd(t *testing.T, cfgPath string, args ...string) (string, error) {
	t.Helper()
	previous := shared.CfgFile
	shared.CfgFile = cfgPath
	t.Cleanup(func() { shared.CfgFile = previous })

	cmd := newConfigCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func readConfigFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	return string(data)
}

func TestConfigGetNestedKey(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	out, err := runConfigCmd(t, path, "get", "defaults.image.model")
	if err != nil {
		t.Fatalf("config get error = %v", err)
	}
	if got := strings.TrimSpace(out); got != "agnes-image-2.5-flash" {
		t.Errorf("config get output = %q", got)
	}
}

func TestConfigGetMasksAPIKey(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	out, err := runConfigCmd(t, path, "get", "api_key")
	if err != nil {
		t.Fatalf("config get error = %v", err)
	}
	if strings.Contains(out, "sk-1234567890abcd") {
		t.Errorf("api_key leaked in full: %q", out)
	}
	if got := strings.TrimSpace(out); got != "...abcd" {
		t.Errorf("masked api_key = %q, want ...abcd", got)
	}

	out, err = runConfigCmd(t, path, "get", "providers.demo.api_key")
	if err != nil {
		t.Fatalf("config get provider key error = %v", err)
	}
	if strings.Contains(out, "sk-provider-secret-1234") {
		t.Errorf("provider api_key leaked in full: %q", out)
	}
	if !strings.Contains(out, "...1234") {
		t.Errorf("provider api_key not masked: %q", out)
	}
}

func TestConfigGetSectionMasksSecrets(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	out, err := runConfigCmd(t, path, "get", "providers")
	if err != nil {
		t.Fatalf("config get section error = %v", err)
	}
	if strings.Contains(out, "sk-provider-secret-1234") {
		t.Errorf("provider api_key leaked through section get: %q", out)
	}
	if !strings.Contains(out, "...1234") {
		t.Errorf("provider api_key not masked in section: %q", out)
	}
}

func TestConfigGetUnknownKey(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	_, err := runConfigCmd(t, path, "get", "defaults.image.unknown")
	if err == nil {
		t.Fatal("config get unknown key = nil error")
	}
	if !strings.Contains(err.Error(), "key not found") {
		t.Errorf("config get error = %q, want key not found", err)
	}
}

func TestConfigListMasksSecrets(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	out, err := runConfigCmd(t, path, "list")
	if err != nil {
		t.Fatalf("config list error = %v", err)
	}
	for _, secret := range []string{"sk-1234567890abcd", "sk-provider-secret-1234"} {
		if strings.Contains(out, secret) {
			t.Errorf("secret %q leaked in list output:\n%s", secret, out)
		}
	}
	// Masked values may be quoted by the YAML emitter ("...abcd"), so compare
	// the flattened output.
	flat := strings.NewReplacer(`"`, "", `'`, "").Replace(out)
	if !strings.Contains(flat, "api_key: ...abcd") {
		t.Errorf("api_key not masked in list output:\n%s", out)
	}
	if !strings.Contains(flat, "...1234") {
		t.Errorf("provider api_key not masked in list output:\n%s", out)
	}
}

func TestConfigMissingFileNeverCreated(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "config.yaml")

	for _, args := range [][]string{
		{"get", "api_key"},
		{"set", "defaults.image.model", "gpt-image-2"},
	} {
		_, err := runConfigCmd(t, missing, args...)
		if err == nil {
			t.Fatalf("config %v = nil error, want file-not-found", args)
		}
		if !strings.Contains(err.Error(), "config file not found") {
			t.Errorf("config %v error = %q, want config file not found", args, err)
		}
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Error("missing config file was created")
	}
}

func TestConfigListMissingFileShowsDefaults(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "config.yaml")

	out, err := runConfigCmd(t, missing, "list")
	if err != nil {
		t.Fatalf("config list error = %v", err)
	}
	if !strings.Contains(out, "config file not found") {
		t.Errorf("list output does not mention the missing file:\n%s", out)
	}
	if !strings.Contains(out, "base_url: https://api.apimart.ai") {
		t.Errorf("list output does not show code defaults:\n%s", out)
	}
}

func TestConfigParentHelp(t *testing.T) {
	path := writeCmdConfig(t, configTestFixture)

	out, err := runConfigCmd(t, path)
	if err != nil {
		t.Fatalf("config help error = %v", err)
	}
	for _, sub := range []string{"get", "set", "list"} {
		if !strings.Contains(out, sub) {
			t.Errorf("config help does not mention %q:\n%s", sub, out)
		}
	}
}
