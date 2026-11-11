package redact

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestRuleEngineDetectsBuiltins(t *testing.T) {
	engine, err := NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	text := "原告张伟，身份证号110105198503121234，电话13812345678，邮箱zhangwei@example.com，案号（2024）京0105民初12345号。"
	result := engine.Detect(NewDocument("t", text))
	if len(result.Spans) != 4 {
		t.Fatalf("got %d spans: %+v", len(result.Spans), result.Spans)
	}
	want := map[string]bool{"ID_CARD": true, "PHONE": true, "EMAIL": true, "CASE_NO": true}
	for _, s := range result.Spans {
		if !want[s.Type] {
			t.Errorf("unexpected type %s (%q)", s.Type, s.Text)
		}
	}
}

func TestRuleEngineFullWidthPhone(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	result := engine.Detect(NewDocument("t", "电话１３８１２３４５６７８。"))
	if len(result.Spans) != 1 || result.Spans[0].Type != "PHONE" {
		t.Fatalf("full-width phone not detected: %+v", result.Spans)
	}
	if got := result.Spans[0].Text; got != "１３８１２３４５６７８" {
		t.Errorf("span text = %q, want original full-width text", got)
	}
}

func TestRuleEngineURLStopsAtCJKPunctuation(t *testing.T) {
	engine, err := NewRuleEngine(&types.RedactConfig{Rules: map[string]bool{"url": true}})
	if err != nil {
		t.Fatal(err)
	}
	result := engine.Detect(NewDocument("t", "网址 https://example.com/a?b=1。"))
	var got string
	for _, span := range result.Spans {
		if span.Type == "URL" {
			got = span.Text
		}
	}
	if got != "https://example.com/a?b=1" {
		t.Fatalf("URL span = %q, want no trailing CJK punctuation", got)
	}
}

func TestRuleEngineToggleAndCustom(t *testing.T) {
	on := false
	cfg := &types.RedactConfig{
		Rules: map[string]bool{"id_card": on},
		Custom: []types.RedactRule{{
			Name: "contract_no_custom", Type: "CONTRACT_NO",
			Regex: `HT-[0-9]{4}-[0-9]{6}`, Enabled: &[]bool{true}[0],
		}},
	}
	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	result := engine.Detect(NewDocument("t", "身份证110105198503121234，合同HT-2024-123456"))
	for _, s := range result.Spans {
		if s.Type == "ID_CARD" {
			t.Fatal("id_card should be disabled")
		}
	}
	if len(result.Spans) != 1 || result.Spans[0].Type != "CONTRACT_NO" {
		t.Fatalf("custom rule not applied: %+v", result.Spans)
	}
}

func TestRuleEngineAllowDeny(t *testing.T) {
	cfg := &types.RedactConfig{
		Allow: []string{"13812345678"},
		Deny:  []string{"某某项目"},
	}
	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	result := engine.Detect(NewDocument("t", "电话13812345678，项目某某项目"))
	if len(result.Spans) != 1 || result.Spans[0].Source != SourceDeny {
		t.Fatalf("allow/deny failed: %+v", result.Spans)
	}
}

func TestRuleEngineRejectsBadConfig(t *testing.T) {
	cases := []*types.RedactConfig{
		{Rules: map[string]bool{"nope": true}},
		{Overrides: map[string]*types.RedactRulePatch{"nope": {}}},
		{Custom: []types.RedactRule{{Name: "id_card", Regex: "x"}}},
		{Custom: []types.RedactRule{{Name: "bad", Regex: "("}}},
		{Allow: []string{"x"}, Deny: []string{"x"}},
	}
	for i, cfg := range cases {
		if _, err := NewRuleEngine(cfg); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestMergeSpansPrefersPriorityAndLength(t *testing.T) {
	spans := []Span{
		{Start: 0, End: 11, Type: "PHONE", Priority: 70},
		{Start: 0, End: 18, Type: "ID_CARD", Priority: 100},
		{Start: 20, End: 24, Type: "EMAIL", Priority: 80},
	}
	got := MergeSpans(spans)
	if len(got) != 2 {
		t.Fatalf("got %d spans, want 2: %+v", len(got), got)
	}
	if got[0].Type != "ID_CARD" {
		t.Errorf("winner = %s, want ID_CARD", got[0].Type)
	}
}

func TestMergeSpansPrefersContainingOrgOverPhone(t *testing.T) {
	spans := []Span{
		{Start: 0, End: 11, Type: "PHONE", Priority: 70},
		{Start: 0, End: 20, Type: "ORG", Priority: 55},
	}
	got := MergeSpans(spans)
	if len(got) != 1 || got[0].Type != "ORG" {
		t.Fatalf("got %+v, want the containing ORG", got)
	}
}

func TestCanonicalType(t *testing.T) {
	cases := map[string]string{
		"人名": "PERSON", "组织": "ORG", "地址": "ADDRESS", "电话": "PHONE",
		"身份证号": "ID_CARD", "统一社会信用代码": "USCC", "B-PERSON": "PERSON",
		"ORGANIZATION": "ORG", "STREET_ADDRESS": "ADDRESS",
	}
	for in, want := range cases {
		if got := CanonicalType(in); got != want {
			t.Errorf("CanonicalType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilterAllowSubstring(t *testing.T) {
	spans := []Span{{Text: "中国工商银行北京朝阳支行", Type: "ORG", Source: SourceRule}}
	if got := filterAllow(spans, []string{"中国工商银行"}, nil); len(got) != 0 {
		t.Fatalf("expected allow to drop the org span, got %v", got)
	}
	_ = strings.TrimSpace("")
}

func TestRuleEngineExpandedCatalog(t *testing.T) {
	engine, err := NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	text := `合同编号：HT-2024-123456
公文文号：国办发〔2024〕1号
保单号：P20240001
病历号：M123456
工号：EMP001
发动机号：EN123456
车架号：LSVAB2BM5EN123456
微信号：wx_abc123
QQ号：12345678
邮编：100020
发票代码：123456789012
股票代码：600000
银行账号：622202020001234567890123
不动产证号：京(2024)1234567
建设工程规划许可证号：京建规(2024)123
著作权登记号：2024-F-001
护照号：E12345678
港澳通行证：C12345678
律师执业证号：11101201911234567
工商注册号：123456789012345
证件号：ABC123456
JP对公账户：JP123456
MAC：00:11:22:33:44:55
车牌号：京A12345
`
	result := engine.Detect(NewDocument("t", text))
	found := map[string]bool{}
	for _, s := range result.Spans {
		found[s.Rule] = true
	}
	want := []string{
		"contract_no", "doc_no", "policy_no", "medical_no", "employee_no",
		"engine_no", "vin", "wechat", "qq", "postcode", "invoice_code",
		"stock_code", "bank_account", "property_cert_no", "construction_permit_no",
		"copyright_no", "passport_ctx", "travel_permit", "lawyer_no_ctx",
		"business_reg_no", "cert_no", "jp_corporate_account", "mac", "plate",
	}
	for _, name := range want {
		if !found[name] {
			t.Errorf("rule %s did not match; spans=%+v", name, result.Spans)
		}
	}
}

func TestDateAndAmountRulesOptIn(t *testing.T) {
	on := true
	cfg := &types.RedactConfig{Rules: map[string]bool{"date_cn": on, "amount": on}}
	engine, err := NewRuleEngine(cfg)
	if err != nil {
		t.Fatal(err)
	}
	result := engine.Detect(NewDocument("t", "日期2024年3月15日，金额：人民币1,250,000元。"))
	found := map[string]bool{}
	for _, s := range result.Spans {
		found[s.Type] = true
	}
	if !found["DATE"] || !found["AMOUNT"] {
		t.Fatalf("date/amount not detected: %+v", result.Spans)
	}
}
