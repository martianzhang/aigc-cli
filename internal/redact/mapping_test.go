package redact

import (
	"strings"
	"testing"
)

func TestApplyRestoreRoundTrip(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "原告张伟，电话13812345678，身份证110105198503121234。"
	doc := NewDocument("t", text)
	spans := engine.Detect(doc).Spans
	masks, _ := NewMaskResolver(nil)
	out, m, err := Apply(doc, spans, ApplyOptions{Style: "placeholder", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "13812345678") || strings.Contains(out, "110105198503121234") {
		t.Fatalf("original values remain: %s", out)
	}
	if !strings.Contains(out, "<PHONE_1>") || !strings.Contains(out, "<ID_CARD_1>") {
		t.Fatalf("missing placeholders: %s", out)
	}
	restored, err := Restore(out, m, false)
	if err != nil {
		t.Fatal(err)
	}
	if restored != text {
		t.Fatalf("restored = %q, want %q", restored, text)
	}
}

func TestApplyAliasesSharePlaceholder(t *testing.T) {
	doc := NewDocument("t", "张伟与老张是同一人，张伟签字。")
	spans := []Span{
		{Start: 0, End: len("张伟"), Type: "PERSON", Text: "张伟", Priority: 100},
		{Start: len("张伟与"), End: len("张伟与老张"), Type: "PERSON", Text: "老张", Priority: 100},
	}
	masks, _ := NewMaskResolver(nil)
	out, m, err := Apply(doc, spans, ApplyOptions{
		Style: "placeholder", Masks: masks,
		Aliases: map[string][]string{"张伟": {"老张"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "<PERSON_1>") != 2 {
		t.Fatalf("aliases should share <PERSON_1>: %s", out)
	}
	if len(m.Entities) != 1 || m.Entities[0].Count != 2 {
		t.Fatalf("entities = %+v", m.Entities)
	}
}

func TestApplyDateAmountDefaultsOn(t *testing.T) {
	doc := NewDocument("t", "日期2024年3月15日，金额人民币1,000元。")
	spans := []Span{
		{Start: len("日期"), End: len("日期2024年3月15日"), Type: "DATE", Text: "2024年3月15日"},
		{Start: len("日期2024年3月15日，金额"), End: len("日期2024年3月15日，金额人民币1,000元"), Type: "AMOUNT", Text: "人民币1,000元"},
	}
	masks, _ := NewMaskResolver(nil)
	out, _, err := Apply(doc, spans, ApplyOptions{Style: "placeholder", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "<DATE_1>") || !strings.Contains(out, "<AMOUNT_1>") {
		t.Fatalf("date/amount should be redacted by default: %s", out)
	}
	out, _, err = Apply(doc, spans, ApplyOptions{Style: "placeholder", Masks: masks, SkipDate: true, SkipAmount: true})
	if err != nil {
		t.Fatal(err)
	}
	if out != doc.Text {
		t.Fatalf("SkipDate/SkipAmount should keep the original: %s", out)
	}
}

func TestRestoreRejectsMaskStyle(t *testing.T) {
	doc := NewDocument("t", "电话13812345678")
	spans := []Span{{Start: len("电话"), End: len("电话13812345678"), Type: "PHONE", Text: "13812345678"}}
	masks, _ := NewMaskResolver(nil)
	out, m, err := Apply(doc, spans, ApplyOptions{Style: "mask", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(out, m, true); err == nil {
		t.Fatal("expected restore to reject mask style")
	}
}

func TestCheckFindsResidualAndAlias(t *testing.T) {
	m := &Mapping{
		Entities: []Entity{{
			Type: "PERSON", Value: "张伟", Placeholder: "<PERSON_1>",
			Aliases: []string{"老张"},
		}},
	}
	res := Check("老张和13812345678", m)
	if len(res) != 1 || res[0].Value != "老张" || res[0].Start != 0 {
		t.Fatalf("residuals = %+v", res)
	}
}

func TestMappingSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/m.json"
	m := &Mapping{Version: 1, Style: "placeholder", Entities: []Entity{{Type: "PHONE", Value: "138", Placeholder: "<PHONE_1>"}}}
	if err := SaveMapping(path, m, false); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMapping(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entities) != 1 || got.Entities[0].Value != "138" {
		t.Fatalf("loaded = %+v", got)
	}
}

func TestApplyIsIdempotent(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "电话13812345678，身份证110105198503121234。"
	doc := NewDocument("t", text)
	spans := engine.Detect(doc).Spans
	masks, _ := NewMaskResolver(nil)
	out, _, err := Apply(doc, spans, ApplyOptions{Style: "placeholder", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	second := engine.Detect(NewDocument("t2", out)).Spans
	if len(second) != 0 {
		t.Fatalf("redacted output still matches rules: %+v", second)
	}
}

func TestApplyPreservesCRLF(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "第一行\r\n电话13812345678\r\n第三行\r\n"
	doc := NewDocument("t", text)
	spans := engine.Detect(doc).Spans
	masks, _ := NewMaskResolver(nil)
	out, _, err := Apply(doc, spans, ApplyOptions{Style: "placeholder", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\r\n") {
		t.Fatalf("CRLF not preserved: %q", out)
	}
}

func TestApplyUnknownStyle(t *testing.T) {
	doc := NewDocument("t", "13812345678")
	spans := []Span{{Start: 0, End: 11, Type: "PHONE", Text: "13812345678"}}
	masks, _ := NewMaskResolver(nil)
	if _, _, err := Apply(doc, spans, ApplyOptions{Style: "nope", Masks: masks}); err == nil {
		t.Fatal("expected unknown style error")
	}
}

func TestApplyConfigAliasConflict(t *testing.T) {
	doc := NewDocument("t", "x")
	masks, _ := NewMaskResolver(nil)
	_, _, err := Apply(doc, nil, ApplyOptions{Masks: masks, Aliases: map[string][]string{
		"a": {"x"}, "b": {"x"},
	}})
	if err == nil {
		t.Fatal("expected alias conflict error")
	}
}

func TestApplyMasksHideNameLengthAndIdentity(t *testing.T) {
	text := "张三与张三丰，汉族和维吾尔族。张三。"
	doc := NewDocument("t", text)
	mk := func(sub, typ string) Span {
		start := strings.Index(text, sub)
		return Span{Start: start, End: start + len(sub), Type: typ, Text: sub}
	}
	last := strings.LastIndex(text, "张三")
	spans := []Span{
		mk("张三", "PERSON"),
		mk("张三丰", "PERSON"),
		mk("汉族", "ETHNICITY"),
		mk("维吾尔族", "ETHNICITY"),
		{Start: last, End: last + len("张三"), Type: "PERSON", Text: "张三"},
	}
	masks, _ := NewMaskResolver(nil)
	out, _, err := Apply(doc, spans, ApplyOptions{Style: "mask", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "张*") != 3 || strings.Count(out, "*族") != 2 {
		t.Fatalf("mask should keep surname and use a fixed given-name mask, got: %s", out)
	}
	for _, leak := range []string{"张三", "张三丰", "汉族", "维吾尔族", "人1", "族1"} {
		if strings.Contains(out, leak) {
			t.Errorf("mask leaked %q: %s", leak, out)
		}
	}

	out, _, err = Apply(doc, spans, ApplyOptions{Style: "black", Masks: masks})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(out, "██") != 3 || strings.Count(out, "█族") != 2 {
		t.Fatalf("black should use identical fixed masks, got: %s", out)
	}
	for _, leak := range []string{"张三", "张三丰", "汉族", "维吾尔族", "█1", "族1"} {
		if strings.Contains(out, leak) {
			t.Errorf("black leaked %q: %s", leak, out)
		}
	}
}
