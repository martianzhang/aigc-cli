package redact

import (
	"context"
	"strings"
	"testing"
)

func TestLegalCorpusCoverage(t *testing.T) {
	engine, err := NewRuleEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	text := `原告：星测影视文化传媒有限公司（以下简称星测公司），被告杭州云测网络科技有限公司（以下简称云测公司）、陈测国。
被告：陈测国，男，1985年3月12日出生。陈测国辩称，涉案APP由他人运营。
证人王测娟出庭作证。王测娟的丈夫刘测胜亦知情。
一、被告杭州云测网络科技有限公司于本判决生效之日起十日内赔偿原告星测影视文化传媒有限公司经济损失100000元；
云测公司账号0200004509200123456，访问日志 IP 203.0.113.7。
被告侵害原告作品《月光测例》。依据《中华人民共和国著作权法》第五十二条。`
	result, err := NewPipeline(engine).Detect(context.Background(), NewDocument("t", text))
	if err != nil {
		t.Fatal(err)
	}
	byType := map[string][]string{}
	for _, span := range result.Spans {
		byType[span.Type] = append(byType[span.Type], span.Text)
	}

	for _, want := range []string{"星测影视文化传媒有限公司", "星测公司", "杭州云测网络科技有限公司", "云测公司"} {
		if !containsString(byType["ORG"], want) {
			t.Errorf("ORG missing %q: %v", want, byType["ORG"])
		}
	}
	for _, want := range []string{"陈测国", "王测娟", "刘测胜"} {
		if countString(byType["PERSON"], want) == 0 {
			t.Errorf("PERSON missing %q: %v", want, byType["PERSON"])
		}
	}
	if countString(byType["PERSON"], "陈测国") < 2 || countString(byType["PERSON"], "王测娟") < 2 {
		t.Errorf("repeated names were not propagated: %v", byType["PERSON"])
	}
	if !containsString(byType["BANK_ACCOUNT"], "0200004509200123456") {
		t.Errorf("bank account missing: %v", byType["BANK_ACCOUNT"])
	}
	if !containsString(byType["IP"], "203.0.113.7") {
		t.Errorf("IP missing: %v", byType["IP"])
	}
	if !containsString(byType["WORK"], "《月光测例》") {
		t.Errorf("work title missing: %v", byType["WORK"])
	}
	for _, span := range result.Spans {
		if strings.Contains(span.Text, "中华人民共和国著作权法") {
			t.Errorf("law title was redacted: %+v", span)
		}
		if strings.Contains(span.Text, "于本判决生效之日起十日内赔偿原告") {
			t.Errorf("org rule swallowed the sentence: %+v", span)
		}
	}
}

func countString(values []string, want string) int {
	n := 0
	for _, value := range values {
		if value == want {
			n++
		}
	}
	return n
}
