package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestLoadRedactConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
defaults:
  redact:
    provider: ollama
    model: has4
    style: mask
    engine: auto
    rules:
      bank_card: false
    custom:
      - name: contract_no_custom
        type: CONTRACT_NO
        regex: 'HT-[0-9]{4}'
        style: mask
    aliases:
      "北京云创智能科技有限公司": ["云创智能"]
    dictionary:
      - value: "某某项目"
        type: PROJECT
    dictionary_files:
      - "./dict/orgs.tsv"
    model_engine: tokencls
    tokencls:
      model: zh-pii
      max_len: 256
      stride: 64
    mask:
      overrides:
        PHONE:
          keep_prefix: 4
          keep_suffix: 4
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults == nil || cfg.Defaults.Redact == nil {
		t.Fatal("defaults.redact section missing")
	}
	rc := cfg.Defaults.Redact
	if rc.Style != "mask" || rc.Engine != "auto" {
		t.Fatalf("redact = %+v", rc)
	}
	if rc.Rules["bank_card"] != false {
		t.Fatalf("rules = %+v", rc.Rules)
	}
	if len(rc.Custom) != 1 || rc.Custom[0].Name != "contract_no_custom" {
		t.Fatalf("custom = %+v", rc.Custom)
	}
	if len(rc.Dictionary) != 1 || rc.Dictionary[0].Type != "PROJECT" {
		t.Fatalf("dictionary = %+v", rc.Dictionary)
	}
	if len(rc.DictionaryFiles) != 1 || rc.DictionaryFiles[0] != "./dict/orgs.tsv" {
		t.Fatalf("dictionary_files = %+v", rc.DictionaryFiles)
	}
	if rc.ModelEngine != "tokencls" || rc.Tokencls == nil ||
		rc.Tokencls.Model != "zh-pii" || rc.Tokencls.MaxLen != 256 {
		t.Fatalf("tokencls = %+v", rc.Tokencls)
	}
	var phone *types.RedactMaskPolicy
	for k, v := range rc.Mask.Overrides {
		if strings.EqualFold(k, "PHONE") {
			phone = v
		}
	}
	if phone == nil || phone.KeepPrefix != 4 || phone.KeepSuffix != 4 {
		t.Fatalf("mask override = %+v", rc.Mask.Overrides)
	}
	if rc.Provider != "ollama" || rc.Model != "has4" {
		t.Fatalf("provider/model = %q/%q", rc.Provider, rc.Model)
	}
}
