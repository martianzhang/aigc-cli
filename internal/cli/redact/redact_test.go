package redact

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func runCommand(t *testing.T, deps Deps, args ...string) (string, error) {
	t.Helper()
	cmd := NewCommand(func() Deps { return deps })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestApplyRestoreCheckFlow(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	text := "原告张伟，电话13812345678，身份证110105198503121234。\n"
	if err := os.WriteFile(src, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	safe := filepath.Join(dir, "a.redacted.txt")
	mapping := filepath.Join(dir, "safe.map.json")
	if _, err := runCommand(t, Deps{OutputDir: dir},
		"apply", src, "--mapping", mapping); err != nil {
		t.Fatal(err)
	}
	redacted, err := os.ReadFile(safe)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(redacted), "<PHONE_1>") || !strings.Contains(string(redacted), "<ID_CARD_1>") {
		t.Fatalf("placeholders missing: %s", redacted)
	}

	restored := filepath.Join(dir, "a.restored.txt")
	if _, err := runCommand(t, Deps{OutputDir: dir},
		"restore", safe, "--mapping", mapping); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(restored)
	if string(got) != text {
		t.Fatalf("restored = %q, want %q", got, text)
	}

	if _, err := runCommand(t, Deps{OutputDir: dir}, "check", safe, "--mapping", mapping); err != nil {
		t.Fatalf("clean check failed: %v", err)
	}

	// Append a leftover phone: check must report it and expose exit code 2.
	if err := os.WriteFile(safe, append(redacted, []byte("漏了13812345678\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = runCommand(t, Deps{OutputDir: dir}, "check", safe, "--mapping", mapping)
	var residual *redactlib.ResidualError
	if !errors.As(err, &residual) {
		t.Fatalf("check error = %v, want ResidualError", err)
	}
	if residual.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2", residual.ExitCode())
	}
}

func TestScanJSONAndDisable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	text := "电话13812345678，身份证110105198503121234。"
	if err := os.WriteFile(src, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCommand(t, Deps{OutputDir: dir}, "scan", src, "--json")
	if err != nil {
		t.Fatal(err)
	}
	var parsed scanOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("scan json: %v\n%s", err, out)
	}
	if parsed.Offset != "rune" || len(parsed.Spans) != 2 {
		t.Fatalf("scan output = %+v", parsed)
	}

	out, err = runCommand(t, Deps{OutputDir: dir}, "scan", src, "--json", "--disable", "id_card")
	if err != nil {
		t.Fatal(err)
	}
	parsed = scanOutput{}
	_ = json.Unmarshal([]byte(out), &parsed)
	for _, s := range parsed.Spans {
		if s.Type == "ID_CARD" {
			t.Fatal("id_card should be disabled")
		}
	}
}

func TestApplyMaskAndSpans(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("电话13812345678"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCommand(t, Deps{OutputDir: dir}, "apply", src, "--stdout", "--style", "mask")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "138****5678") {
		t.Fatalf("mask output = %q", out)
	}

	spans := filepath.Join(dir, "spans.json")
	scanOut, err := runCommand(t, Deps{OutputDir: dir}, "scan", src, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spans, []byte(scanOut), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err = runCommand(t, Deps{OutputDir: dir}, "apply", src, "--stdout", "--spans", spans)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<PHONE_1>") {
		t.Fatalf("spans apply output = %q", out)
	}
}

func TestRulesFileAndEnable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("电话13812345678，合同HT-2024-123456。"), 0o644); err != nil {
		t.Fatal(err)
	}
	rules := filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(rules, []byte(`
custom:
  - name: contract_no_custom
    type: CONTRACT_NO
    regex: 'HT-[0-9]{4}-[0-9]{6}'
`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCommand(t, Deps{OutputDir: dir},
		"scan", src, "--json", "--rules-file", rules,
		"--enable", "contract_no_custom", "--disable", "phone_mobile,contract_no")
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

func TestRemoteModelEndpointRejected(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("电话13812345678"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := Deps{
		OutputDir: dir,
		ResolveProvider: func(string) *provider.EffectiveProvider {
			return &provider.EffectiveProvider{Name: "remote", Type: types.ProviderOpenAI,
				BaseURL: "https://example.com/v1", Model: "mock"}
		},
	}
	if _, err := runCommand(t, deps, "scan", src, "--engine", "model"); err == nil ||
		!strings.Contains(err.Error(), "allow-remote") {
		t.Fatalf("error = %v, want allow-remote hint", err)
	}
}

func TestChatnerProviderWiring(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"组织\":[\"北京云创智能科技有限公司\"]}"}}]}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("被告：北京云创智能科技有限公司，电话13812345678。"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := Deps{
		OutputDir: dir,
		ResolveProvider: func(string) *provider.EffectiveProvider {
			return &provider.EffectiveProvider{
				Name: "mock", Type: types.ProviderOpenAI, BaseURL: srv.URL, Model: "mock",
			}
		},
	}
	out, err := runCommand(t, deps, "scan", src, "--json", "--engine", "model")
	if err != nil {
		t.Fatal(err)
	}
	var parsed scanOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range parsed.Spans {
		if s.Type == "ORG" && strings.Contains(s.Text, "云创") {
			found = true
		}
		if s.Type == "PHONE" {
			t.Fatalf("engine=model must not run the rule engine, got PHONE: %s", out)
		}
	}
	if !found {
		t.Fatalf("model ORG span missing: %s", out)
	}
}

func TestModelEngineOnlyAcceptsBackendNames(t *testing.T) {
	cfg, err := (buildFlags{modelEngine: "chatner"}).redactConfig()
	if err != nil || cfg.ModelEngine != "chatner" {
		t.Fatalf("chatner: cfg=%+v err=%v", cfg, err)
	}
	_, err = (buildFlags{modelEngine: "has4"}).redactConfig()
	if err == nil || !strings.Contains(err.Error(), "--model has4") {
		t.Fatalf("bare model name error = %v, want guidance to --model has4", err)
	}
}

func TestApplyDatesDefaultOn(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("原告：张三，1987年3月12日出生，电话13812345678。\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runCommand(t, Deps{OutputDir: dir}, "apply", src, "--stdout")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<DATE_1>") {
		t.Fatalf("date was not redacted by default:\n%s", out)
	}
	if strings.Contains(out, "1987年3月12日") {
		t.Fatalf("date leaked:\n%s", out)
	}

	out, err = runCommand(t, Deps{OutputDir: dir}, "apply", src, "--stdout", "--disable", "date_cn")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "1987年3月12日") {
		t.Fatalf("--disable date_cn did not keep the date:\n%s", out)
	}
}
