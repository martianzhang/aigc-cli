package redact

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func maskResolver(t *testing.T, cfg *types.RedactConfig) *MaskResolver {
	t.Helper()
	r, err := NewMaskResolver(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMaskDefaults(t *testing.T) {
	r := maskResolver(t, nil)
	card := luhnComplete("622202020001234")
	spaced := card[0:4] + " " + card[4:8] + " " + card[8:12] + " " + card[12:16]
	cases := []struct {
		typ, value, want string
	}{
		{"PHONE", "13812345678", "138****5678"},
		{"ID_CARD", "110105198503121234", "110105********1234"},
		{"BANK_CARD", spaced, "**** **** **** " + card[12:16]},
		{"PERSON", "张伟", "张*"},
		{"PERSON", "欧阳娜娜", "欧阳**"},
		{"PERSON", "Zhang Wei", "Z**** W**"},
		{"EMAIL", "zhangwei@example.com", "z*******@example.com"},
		{"ADDRESS", "北京市朝阳区建国路88号", "北京市朝阳区******"},
	}
	for _, c := range cases {
		if got := MaskValue(c.value, c.typ, r.Policy(c.typ)); got != c.want {
			t.Errorf("MaskValue(%q, %s) = %q, want %q", c.value, c.typ, got, c.want)
		}
	}
}

func TestMaskPositions(t *testing.T) {
	base := MaskPolicy{Strategy: "fixed", Char: "*", MaskLen: 3, PreserveLength: true, KeepSeparators: true, KeepPrefix: 3, KeepSuffix: 4}
	cases := []struct {
		pos  string
		want string
	}{
		{"middle", "138****5678"},
		{"suffix", "138********"},
		{"prefix", "*******5678"},
		{"all", "***********"},
	}
	for _, c := range cases {
		p := base
		p.Position = c.pos
		if got := MaskValue("13812345678", "PHONE", p); got != c.want {
			t.Errorf("position %s: got %q, want %q", c.pos, got, c.want)
		}
	}
}

func TestMaskConfigOverride(t *testing.T) {
	r := maskResolver(t, &types.RedactConfig{Mask: &types.RedactMaskPolicy{
		Overrides: map[string]*types.RedactMaskPolicy{
			"ID_CARD": {KeepPrefix: 2, KeepSuffix: 2},
		},
	}})
	got := MaskValue("110105198503121234", "ID_CARD", r.Policy("ID_CARD"))
	if got != "11**************34" {
		t.Fatalf("override mask = %q", got)
	}
}

func TestMaskRejectsBadPolicy(t *testing.T) {
	if _, err := NewMaskResolver(&types.RedactConfig{Mask: &types.RedactMaskPolicy{Strategy: "nope"}}); err == nil {
		t.Fatal("expected unknown strategy error")
	}
	if _, err := NewMaskResolver(&types.RedactConfig{Mask: &types.RedactMaskPolicy{Position: "nope"}}); err == nil {
		t.Fatal("expected unknown position error")
	}
	if _, err := NewMaskResolver(&types.RedactConfig{Mask: &types.RedactMaskPolicy{KeepPrefixRegex: "("}}); err == nil {
		t.Fatal("expected invalid regex error")
	}
}

func TestMaskPreserveLengthFalse(t *testing.T) {
	p := MaskPolicy{Strategy: "fixed", Position: "middle", KeepPrefix: 3, KeepSuffix: 4,
		Char: "*", MaskLen: 2, PreserveLength: false, KeepSeparators: true}
	if got := MaskValue("13812345678", "PHONE", p); got != "138**5678" {
		t.Fatalf("fixed length mask = %q", got)
	}
}

func TestMaskKeepsAtLeastOneChar(t *testing.T) {
	p := MaskPolicy{Strategy: "fixed", Position: "middle", KeepPrefix: 99, KeepSuffix: 99,
		Char: "*", MaskLen: 3, PreserveLength: true}
	got := MaskValue("张伟", "PERSON", p)
	if !strings.Contains(got, "*") {
		t.Fatalf("expected at least one mask char, got %q", got)
	}
}
