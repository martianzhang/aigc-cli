package redact

import (
	"context"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

type fakeEngine struct {
	result Result
}

func (f fakeEngine) Name() string { return "fake" }
func (f fakeEngine) Detect(context.Context, *Document) (Result, error) {
	return f.result, nil
}

func TestPipelineMergesAndValidatesModelSpans(t *testing.T) {
	doc := NewDocument("t", "电话13812345678，身份证110105198503121234，机构北京云创智能科技有限公司。")
	rules, _ := NewRuleEngine(nil)
	org := "北京云创智能科技有限公司"
	orgStart := strings.Index(doc.Text, org)
	model := fakeEngine{result: Result{Spans: []Span{
		{Start: 0, End: 5, Type: "ID_CARD", Text: "电话138"}, // invalid checksum -> dropped
		{Start: orgStart, End: orgStart + len(org), Type: "ORG", Text: org},
	}}}
	p := NewPipeline(rules, model)
	result, err := p.Detect(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range result.Spans {
		if s.Type == "ID_CARD" && strings.HasPrefix(s.Text, "电话") {
			t.Fatalf("invalid model ID span survived: %+v", s)
		}
	}
	if len(result.Spans) != 3 { // phone + ID + ORG; invalid model span dropped
		t.Fatalf("spans = %+v", result.Spans)
	}
}

func TestRuleEngineRulesAndPatch(t *testing.T) {
	group := 1
	priority := 120
	maxLen := 30
	on := false
	cfg := &types.RedactConfig{
		Overrides: map[string]*types.RedactRulePatch{
			"email": {Regex: `([A-Za-z]+)@[A-Za-z]+\.com`, Group: &group, Priority: &priority, MaxLen: &maxLen, Enabled: &on},
		},
	}
	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	infos := engine.Rules()
	found := false
	for _, info := range infos {
		if info.Name == "email" {
			found = true
			if info.Enabled || info.Priority != 120 || info.Group != 1 || info.MaxLen != 30 {
				t.Fatalf("email patch = %+v", info)
			}
		}
	}
	if !found {
		t.Fatal("email rule missing")
	}
	if len(BuiltinRuleNames()) == 0 {
		t.Fatal("expected built-in names")
	}
}

func TestValidatorsIPAndURL(t *testing.T) {
	if !ValidateIP("192.168.1.1") || !ValidateIP("::1") {
		t.Fatal("expected valid IPs")
	}
	if ValidateIP("999.1.1.1") {
		t.Fatal("expected invalid IP")
	}
	if !ValidateURL("https://example.com/a") {
		t.Fatal("expected valid URL")
	}
	if ValidateURL("file:///tmp/a") || ValidateURL("not a url") {
		t.Fatal("expected invalid URL")
	}
}

func TestApplyHashAndMixed(t *testing.T) {
	doc := NewDocument("t", "电话13812345678，邮箱a@example.com")
	spans := []Span{
		{Start: len("电话"), End: len("电话13812345678"), Type: "PHONE", Text: "13812345678", Style: "hash"},
		{Start: len("电话13812345678，邮箱"), End: len("电话13812345678，邮箱a@example.com"), Type: "EMAIL", Text: "a@example.com"},
	}
	masks, _ := NewMaskResolver(nil)
	out, m, err := Apply(doc, spans, ApplyOptions{Style: "placeholder", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<PHONE_") {
		t.Fatalf("hash placeholder missing: %s", out)
	}
	if m.Style != "mixed" {
		t.Fatalf("mapping style = %q, want mixed", m.Style)
	}
	if _, err := Restore(out, m, true); err == nil {
		t.Fatal("mixed mapping must not be restorable")
	}
}

func TestRestoreRejectsChangedDocAndForce(t *testing.T) {
	doc := NewDocument("t", "电话13812345678")
	spans := []Span{{Start: len("电话"), End: len("电话13812345678"), Type: "PHONE", Text: "13812345678"}}
	masks, _ := NewMaskResolver(nil)
	out, m, err := Apply(doc, spans, ApplyOptions{Style: "placeholder", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(out+"x", m, false); err == nil {
		t.Fatal("expected sha mismatch")
	}
	m.OutputSHA256 = ""
	broken := strings.Replace(out, "<PHONE_1>", "<PHONE_9>", 1)
	if _, err := Restore(broken, m, false); err == nil {
		t.Fatal("expected placeholder mismatch")
	}
}

func TestChatnerNameAndFirst(t *testing.T) {
	fc := &fakeChat{content: `{"电话":["13812345678"]}`}
	eng, _ := NewChatner(fc, "has4", &types.RedactChatnerConfig{Occurrence: OccurrenceFirst})
	if eng.Name() != "chatner" {
		t.Fatal("unexpected name")
	}
	text := "电话13812345678，备用电话13812345678。"
	result, err := eng.Detect(context.Background(), NewDocument("t", text))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Spans) != 1 {
		t.Fatalf("first-occurrence mode returned %d spans", len(result.Spans))
	}
}

func TestChatnerCustomPrompt(t *testing.T) {
	fc := &fakeChat{content: `{}`}
	eng, _ := NewChatner(fc, "has4", &types.RedactChatnerConfig{Prompt: "extract: {text}"})
	if got := eng.promptFor("abc"); got != "extract: abc" {
		t.Fatalf("prompt = %q", got)
	}
	eng, _ = NewChatner(fc, "has4", &types.RedactChatnerConfig{Prompt: "extract"})
	if got := eng.promptFor("abc"); !strings.Contains(got, "<text>abc</text>") {
		t.Fatalf("prompt = %q", got)
	}
}

func TestResidualErrorExitCode(t *testing.T) {
	err := &ResidualError{Count: 2}
	if err.ExitCode() != 2 || !strings.Contains(err.Error(), "2") {
		t.Fatalf("error = %v", err)
	}
}
