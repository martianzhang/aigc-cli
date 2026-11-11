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
	if len(result.Spans) != 5 {
		t.Fatalf("got %d spans: %+v", len(result.Spans), result.Spans)
	}
	want := map[string]bool{"PERSON": true, "ID_CARD": true, "PHONE": true, "EMAIL": true, "CASE_NO": true}
	for _, s := range result.Spans {
		if !want[s.Type] {
			t.Errorf("unexpected type %s (%q)", s.Type, s.Text)
		}
		if s.Type == "PERSON" && s.Text != "张伟" {
			t.Errorf("person = %q, want 张伟", s.Text)
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

func TestDateAndAmountRulesDefaultOn(t *testing.T) {
	engine, err := NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	result := engine.Detect(NewDocument("t", "日期2024年3月15日，金额：人民币1,250,000元（伍拾万元整）。"))
	found := map[string]bool{}
	for _, s := range result.Spans {
		found[s.Type] = true
	}
	if !found["DATE"] || !found["AMOUNT"] {
		t.Fatalf("date/amount not detected by default: %+v", result.Spans)
	}

	off := false
	engine, err = NewRuleEngine(&types.RedactConfig{Rules: map[string]bool{
		"date_cn": false, "amount": off, "amount_cn": off,
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, span := range engine.Detect(NewDocument("t", "日期2024年3月15日，金额：人民币1,250,000元。")).Spans {
		if span.Type == "DATE" || span.Type == "AMOUNT" {
			t.Fatalf("date/amount should be disableable: %+v", span)
		}
	}
}

func TestCredentialRules(t *testing.T) {
	engine, err := NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	text := `--host=pc-2ze565vuk61x2zcb0.rwlb.rds.aliyuncs.com --user=root_admin --password=uVLIn2iU0ZgFUU --port=3306`
	result := engine.Detect(NewDocument("t", text))
	got := map[string]string{}
	for _, span := range result.Spans {
		got[span.Type] = span.Text
	}
	if got["HOST"] != "pc-2ze565vuk61x2zcb0.rwlb.rds.aliyuncs.com" {
		t.Errorf("HOST = %q", got["HOST"])
	}
	if got["USERNAME"] != "root_admin" {
		t.Errorf("USERNAME = %q", got["USERNAME"])
	}
	if got["PASSWORD"] != "uVLIn2iU0ZgFUU" {
		t.Errorf("PASSWORD = %q", got["PASSWORD"])
	}
}

func TestDatabaseURLRule(t *testing.T) {
	engine, err := NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	text := `DATABASE_URL=mysql://root:secret@db.internal:3306/app`
	result := engine.Detect(NewDocument("t", text))
	var url string
	for _, span := range result.Spans {
		if span.Type == "DB_URL" {
			url = span.Text
		}
	}
	if url != "mysql://root:secret@db.internal:3306/app" {
		t.Fatalf("DB_URL = %q", url)
	}
}

func TestCredentialMaskHidesPassword(t *testing.T) {
	got := MaskValue("uVLIn2iU0ZgFUU", "PASSWORD", defaultMaskPolicy("PASSWORD"))
	if got != "********" {
		t.Fatalf("password mask = %q, want fixed 8 stars", got)
	}
}

func TestCredentialRulesJSONAndQuotes(t *testing.T) {
	engine, err := NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	text := `{"password":"p@ss","username":"admin","db_host":"db.internal","host":"localhost"}`
	result := engine.Detect(NewDocument("t", text))
	got := map[string]string{}
	for _, span := range result.Spans {
		got[span.Type] = span.Text
	}
	if got["PASSWORD"] != `"p@ss"` {
		t.Errorf("PASSWORD = %q", got["PASSWORD"])
	}
	if got["USERNAME"] != `"admin"` {
		t.Errorf("USERNAME = %q", got["USERNAME"])
	}
	if got["HOST"] != `"db.internal"` {
		t.Errorf("HOST = %q", got["HOST"])
	}
	for _, span := range result.Spans {
		if strings.Contains(span.Text, "localhost") {
			t.Errorf("bare host key should not be redacted: %+v", result.Spans)
		}
	}
}

func TestAuthorizationAndCookieRules(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "Authorization: Bearer abcdefghijklmnop\nCookie: session=abc123; token=xyz"
	result := engine.Detect(NewDocument("t", text))
	want := map[string]string{"CREDENTIAL": "abcdefghijklmnop", "COOKIE": "session=abc123; token=xyz"}
	got := map[string]string{}
	for _, span := range result.Spans {
		got[span.Type] = span.Text
	}
	for typ, value := range want {
		if got[typ] != value {
			t.Errorf("%s = %q, want %q", typ, got[typ], value)
		}
	}
}

func TestKnownAPIKeyAndJWTRules(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n" +
		"token=ghp_1234567890abcdefghijklmnopqrstuvwxyz\n" +
		"jwt eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnop"
	result := engine.Detect(NewDocument("t", text))
	found := map[string]bool{}
	for _, span := range result.Spans {
		found[span.Rule+":"+span.Text] = true
	}
	for _, want := range []string{
		"api_key_known:AKIAIOSFODNN7EXAMPLE",
		"api_key_known:ghp_1234567890abcdefghijklmnopqrstuvwxyz",
		"jwt_token:eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghijklmnop",
	} {
		if !found[want] {
			t.Errorf("missing %s in %+v", want, result.Spans)
		}
	}
}

func TestPrivateKeyBlockRule(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"
	result := engine.Detect(NewDocument("t", text))
	if len(result.Spans) != 1 || result.Spans[0].Type != "PRIVATE_KEY" {
		t.Fatalf("spans = %+v", result.Spans)
	}
}

func TestURLWithCredentialsRule(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "https://alice:s3cret@example.com/path"
	result := engine.Detect(NewDocument("t", text))
	if len(result.Spans) != 1 || result.Spans[0].Type != "URL" {
		t.Fatalf("spans = %+v", result.Spans)
	}
}

func TestADONETAndCLIFormRules(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "Server=db.internal;User Id=sa;Password=P@ssw0rd;\nmysql --password s3cret --user root"
	result := engine.Detect(NewDocument("t", text))
	got := map[string][]string{}
	for _, span := range result.Spans {
		got[span.Type] = append(got[span.Type], span.Text)
	}
	if !containsString(got["HOST"], "db.internal") || !containsString(got["USERNAME"], "sa") ||
		!containsString(got["PASSWORD"], "P@ssw0rd") {
		t.Errorf("ADO.NET spans = %+v", result.Spans)
	}
	if !containsString(got["PASSWORD"], "s3cret") || !containsString(got["USERNAME"], "root") {
		t.Errorf("CLI spans = %+v", result.Spans)
	}
}

func TestCryptoWalletRule(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "0x52908400098527886E0F7030069857D2E4169EE7"
	result := engine.Detect(NewDocument("t", text))
	if len(result.Spans) != 1 || result.Spans[0].Type != "CRYPTO" {
		t.Fatalf("spans = %+v", result.Spans)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCredentialKeyVariants(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "user_id=alice\ncompass=north\nbypass=true"
	result := engine.Detect(NewDocument("t", text))
	var user string
	for _, span := range result.Spans {
		if span.Type == "USERNAME" {
			user = span.Text
		}
		if span.Type == "PASSWORD" {
			t.Errorf("unexpected password span for %q", span.Text)
		}
	}
	if user != "alice" {
		t.Fatalf("user_id not redacted: %+v", result.Spans)
	}
}

func TestAPIKeyAssignType(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := `{"apiKey":"secret-value","PIXABAY_API_KEY":"12345678","UNSPLASH_ACCESS_KEY":"abcdefgh"}`
	result := engine.Detect(NewDocument("t", text))
	if len(result.Spans) != 3 {
		t.Fatalf("spans = %+v", result.Spans)
	}
	for _, span := range result.Spans {
		if span.Type != "API_KEY" || span.Rule != "api_key_assign" {
			t.Errorf("span = %+v, want API_KEY/api_key_assign", span)
		}
	}
}

func TestFictionalIDCardRules(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "公民身份号码320104198703125517，另一个320105197911083316。"
	result := engine.Detect(NewDocument("t", text))
	var ids []string
	for _, span := range result.Spans {
		if span.Type == "BANK_CARD" {
			t.Errorf("ID-shaped value classified as BANK_CARD: %+v", span)
		}
		if span.Type == "ID_CARD" {
			ids = append(ids, span.Text)
		}
	}
	if len(ids) != 2 || ids[0] != "320104198703125517" || ids[1] != "320105197911083316" {
		t.Fatalf("ID spans = %+v", result.Spans)
	}
}

func TestOrgSuffixRule(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	text := "被告一：南京启衡建材贸易有限公司，法定代表人：高振坤。"
	result := engine.Detect(NewDocument("t", text))
	found := false
	for _, span := range result.Spans {
		if span.Type == "ORG" && span.Text == "南京启衡建材贸易有限公司" {
			found = true
		}
	}
	if !found {
		t.Fatalf("org not detected: %+v", result.Spans)
	}
}

func TestExtendAddressSpanIncludesBuildingAndRoom(t *testing.T) {
	doc := NewDocument("t", "住江苏省南京市鼓楼区中央路128号3幢802室，联系电话13851700286。")
	start := strings.Index(doc.Text, "江苏省")
	end := strings.Index(doc.Text, "128号") + len("128号")
	span := Span{Start: start, End: end, Type: "ADDRESS", Text: doc.Text[start:end]}
	got := extendAddressSpan(doc, span)
	if got.Text != "江苏省南京市鼓楼区中央路128号3幢802室" {
		t.Fatalf("extended address = %q", got.Text)
	}
}

func TestNameAssignRule(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	result := engine.Detect(NewDocument("t", "姓名：张三，曾用名：李四。"))
	if len(result.Spans) != 2 {
		t.Fatalf("spans = %+v", result.Spans)
	}
	for _, span := range result.Spans {
		if span.Type != "PERSON" {
			t.Errorf("span = %+v", span)
		}
	}
}

func TestAdditionalBusinessRules(t *testing.T) {
	engine, _ := NewRuleEngine(&types.RedactConfig{Rules: map[string]bool{"amount_cn": true}})
	text := "税号：123456789012345\n项目编号：PRJ-2024-001\n人民币伍拾万元整\nMr. John Smith"
	result := engine.Detect(NewDocument("t", text))
	got := map[string]string{}
	for _, span := range result.Spans {
		got[span.Type] = span.Text
	}
	if got["TAX_ID"] != "123456789012345" {
		t.Errorf("TAX_ID = %q", got["TAX_ID"])
	}
	if got["PROJECT_NO"] != "PRJ-2024-001" {
		t.Errorf("PROJECT_NO = %q", got["PROJECT_NO"])
	}
	if got["AMOUNT"] != "伍拾万元整" {
		t.Errorf("AMOUNT = %q", got["AMOUNT"])
	}
	if got["PERSON"] != "John Smith" {
		t.Errorf("PERSON = %q", got["PERSON"])
	}
}

func TestEthnicityRules(t *testing.T) {
	engine, _ := NewRuleEngine(nil)
	result := engine.Detect(NewDocument("t", "男，汉族，民族：回族。汉族和维吾尔族。水族箱"))
	found := map[string]bool{}
	for _, span := range result.Spans {
		if span.Type == "ETHNICITY" {
			found[span.Text] = true
		}
	}
	if !found["汉族"] || !found["回族"] {
		t.Fatalf("ethnicity spans = %+v", result.Spans)
	}
	if found["水族"] {
		t.Fatalf("水族箱 should not be treated as ethnicity: %+v", result.Spans)
	}
}

func TestAgeSSNAndInternationalPhoneRules(t *testing.T) {
	engine, err := NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	result := engine.Detect(NewDocument("t", "年龄：35岁，SSN 123-45-6789，电话 +1 (415) 555-0123。"))
	found := map[string]string{}
	for _, s := range result.Spans {
		found[s.Type] = s.Text
	}
	if found["AGE"] != "35" {
		t.Errorf("AGE = %q, spans = %+v", found["AGE"], result.Spans)
	}
	if found["SSN"] != "123-45-6789" {
		t.Errorf("SSN = %q, spans = %+v", found["SSN"], result.Spans)
	}
	if !strings.Contains(found["PHONE"], "415") || !strings.Contains(found["PHONE"], "0123") {
		t.Errorf("PHONE = %q, spans = %+v", found["PHONE"], result.Spans)
	}
}

func TestEnglishAddressAndIDRules(t *testing.T) {
	engine, err := NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	text := "Address: 123 Main Street, Apt 4B. Passport No: AB1234567. NI AB 12 34 56 C. Driver's License: D1234567."
	result := engine.Detect(NewDocument("t", text))
	found := map[string]string{}
	for _, s := range result.Spans {
		found[s.Type] = s.Text
	}
	if !strings.Contains(found["ADDRESS"], "123 Main Street") {
		t.Errorf("ADDRESS = %q, spans = %+v", found["ADDRESS"], result.Spans)
	}
	if found["PASSPORT"] != "AB1234567" {
		t.Errorf("PASSPORT = %q, spans = %+v", found["PASSPORT"], result.Spans)
	}
	if !strings.Contains(found["NATIONAL_ID"], "AB 12 34 56 C") {
		t.Errorf("NATIONAL_ID = %q, spans = %+v", found["NATIONAL_ID"], result.Spans)
	}
	if found["DRIVER_LICENSE"] != "D1234567" {
		t.Errorf("DRIVER_LICENSE = %q, spans = %+v", found["DRIVER_LICENSE"], result.Spans)
	}
}
