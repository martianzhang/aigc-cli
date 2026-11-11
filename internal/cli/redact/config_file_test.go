package redact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestMergeRedactFileDeepMergesAndUnwraps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "strict.yaml")
	overlay := `
defaults:
  redact:
    style: mask
    rules:
      phone_mobile: false
    custom:
      - name: contract_no_custom
        type: CONTRACT_NO
        regex: 'HT-[0-9]{4}'
`
	if err := os.WriteFile(path, []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &types.RedactConfig{
		Style:  "placeholder",
		Engine: "rules",
		Rules:  map[string]bool{"id_card": true, "phone_mobile": true},
	}
	if err := mergeRedactFile(cfg, path); err != nil {
		t.Fatal(err)
	}
	if cfg.Style != "mask" {
		t.Fatalf("style = %q, want mask", cfg.Style)
	}
	if cfg.Engine != "rules" {
		t.Fatalf("engine = %q, want inherited rules", cfg.Engine)
	}
	if !cfg.Rules["id_card"] {
		t.Fatalf("rules lost id_card: %+v", cfg.Rules)
	}
	if cfg.Rules["phone_mobile"] {
		t.Fatalf("rules phone_mobile = true, want overridden false: %+v", cfg.Rules)
	}
	if len(cfg.Custom) != 1 || cfg.Custom[0].Name != "contract_no_custom" {
		t.Fatalf("custom = %+v", cfg.Custom)
	}
}

func TestMergeRedactFileAcceptsBareRedactSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bare.yaml")
	if err := os.WriteFile(path, []byte("engine: auto\nstyle: black\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &types.RedactConfig{Engine: "rules", Style: "placeholder"}
	if err := mergeRedactFile(cfg, path); err != nil {
		t.Fatal(err)
	}
	if cfg.Engine != "auto" || cfg.Style != "black" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestProfileSwitchesExternalRules(t *testing.T) {
	dir := t.TempDir()
	mainConfig := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(mainConfig, []byte("defaults:\n  redact: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	strict := filepath.Join(dir, "strict.yaml")
	if err := os.WriteFile(strict, []byte(`
rules:
  phone_mobile: false
custom:
  - name: contract_no_custom
    type: CONTRACT_NO
    regex: 'HT-[0-9]{4}'
`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("电话13812345678，合同HT-2024。"), 0o644); err != nil {
		t.Fatal(err)
	}

	prevCfg, prevFile := options.Shared.Cfg, options.Shared.CfgFile
	options.Shared.Cfg = &types.Config{Defaults: &types.ConfigDefaults{
		Redact: &types.RedactConfig{
			Rules:    map[string]bool{},
			Profiles: map[string]string{"strict": "./strict.yaml"},
		},
	}}
	options.Shared.CfgFile = mainConfig
	t.Cleanup(func() {
		options.Shared.Cfg, options.Shared.CfgFile = prevCfg, prevFile
	})

	out, err := runCommand(t, Deps{OutputDir: dir},
		"scan", src, "--json", "--profile", "strict")
	if err != nil {
		t.Fatal(err)
	}
	var parsed scanOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Spans) != 1 || parsed.Spans[0].Type != "CONTRACT_NO" {
		t.Fatalf("spans = %+v", parsed.Spans)
	}
}

func TestUnknownProfileListsAvailable(t *testing.T) {
	_, err := resolveRedactProfile(&types.RedactConfig{
		Profiles: map[string]string{"strict": "./strict.yaml", "loose": "./loose.yaml"},
	}, "nope")
	if err == nil || !strings.Contains(err.Error(), "loose, strict") {
		t.Fatalf("error = %v, want available profiles", err)
	}
}
