package redact

import (
	"path/filepath"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestOrderedLabels(t *testing.T) {
	labels, err := orderedLabels(map[string]string{"0": "O", "2": "B-PERSON", "1": "I-PERSON"})
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != 3 || labels[2] != "B-PERSON" || labels[1] != "I-PERSON" {
		t.Fatalf("labels = %v", labels)
	}
	if _, err := orderedLabels(nil); err == nil {
		t.Fatal("expected empty id2label error")
	}
	if _, err := orderedLabels(map[string]string{"x": "O"}); err == nil {
		t.Fatal("expected invalid key error")
	}
}

func TestDecodeTokenSpansBIOES(t *testing.T) {
	text := "原告张伟，电话13812345678。"
	doc := NewDocument("t", text)
	// "张"(2,3) "伟"(3,4)  -> PERSON; phone starts at char 7.
	items := []tokenItem{
		{startChar: 2, endChar: 3, id: 1},
		{startChar: 3, endChar: 4, id: 2},
		{startChar: 7, endChar: 8, id: 3},
		{startChar: 8, endChar: 9, id: 4},
		{startChar: 9, endChar: 10, id: 5},
	}
	labels := []string{"B-PERSON", "E-PERSON", "B-PHONE", "I-PHONE", "E-PHONE"}
	spans := decodeTokenSpans(doc, items, labels)
	if len(spans) != 2 {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].Type != "PERSON" || spans[0].Text != "张伟" {
		t.Fatalf("person = %+v", spans[0])
	}
	if spans[1].Type != "PHONE" || spans[1].Text != "138" {
		t.Fatalf("phone = %+v", spans[1])
	}
}

func TestDecodeTokenSpansSingleAndBareLabel(t *testing.T) {
	text := "北京云创智能科技有限公司"
	doc := NewDocument("t", text)
	items := []tokenItem{{startChar: 0, endChar: 2}, {startChar: 2, endChar: 12}}
	spans := decodeTokenSpans(doc, items, []string{"S-ORG", "ORG"})
	if len(spans) != 2 {
		t.Fatalf("spans = %+v", spans)
	}
	if spans[0].Type != "ORG" || spans[1].Type != "ORG" {
		t.Fatalf("types = %+v", spans)
	}
}

func TestSplitTokenLabel(t *testing.T) {
	cases := map[string][2]string{
		"B-PERSON":  {"B", "PERSON"},
		"I-ORG":     {"I", "ORG"},
		"E-ADDRESS": {"E", "ADDRESS"},
		"S-PHONE":   {"S", "PHONE"},
		"O":         {"", ""},
		"PERSON":    {"B", "PERSON"},
	}
	for in, want := range cases {
		tag, ent := splitTokenLabel(in)
		if tag != want[0] || ent != want[1] {
			t.Errorf("splitTokenLabel(%q) = (%q,%q), want (%q,%q)", in, tag, ent, want[0], want[1])
		}
	}
}

func TestTokenClassifierMissingModel(t *testing.T) {
	_, err := NewTokenClassifier(&types.RedactTokenclsConfig{
		ModelDir: filepath.Join(t.TempDir(), "missing"),
	}, t.TempDir())
	if err == nil {
		t.Fatal("expected missing model error")
	}
}

func TestViterbiBIOESRecoversEntity(t *testing.T) {
	labels := []string{"O", "B-PER", "E-PER"}
	// 2 tokens: O is top for the first token, E-PER for the second; only a
	// B->E path can produce the entity.
	logits := []float32{
		0, 0, 0, // CLS row (ignored)
		4.42, 4.20, 0.0,
		4.53, 0.0, 5.21,
	}
	got := viterbiLabels(logits, 2, 3, labels, tokenTransitionBiases{})
	if len(got) != 2 || got[0] != "B-PER" || got[1] != "E-PER" {
		t.Fatalf("viterbi = %v, want [B-PER E-PER]", got)
	}
}

func TestViterbiHighRecallRecoversPrecisionHeavySpan(t *testing.T) {
	labels := []string{"O", "B-PER", "E-PER"}
	// Real privyscope logits for "张伟" inside a longer sentence: O leads on
	// both tokens (10.15 vs 9.51), but the high_recall begin/end bias crosses
	// the gap. This is why plain argmax and zero-bias Viterbi miss the name.
	logits := []float32{
		0, 0, 0, // CLS row (ignored)
		4.98, 4.19, 0.0,
		5.17, 0.0, 5.32,
	}
	recall, _ := resolveTokenBiases(&types.RedactTokenclsConfig{OperatingPoint: "high_recall"})
	got := viterbiLabels(logits, 2, 3, labels, recall)
	if len(got) != 2 || got[0] != "B-PER" || got[1] != "E-PER" {
		t.Fatalf("high_recall viterbi = %v, want [B-PER E-PER]", got)
	}
	balanced, _ := resolveTokenBiases(&types.RedactTokenclsConfig{OperatingPoint: "balanced"})
	if got := viterbiLabels(logits, 2, 3, labels, balanced); got[0] != "O" || got[1] != "O" {
		t.Fatalf("balanced viterbi = %v, want [O O]", got)
	}
}
