package redact

import (
	"context"
	"testing"
)

func FuzzPipelineDetect(f *testing.F) {
	f.Add("联系方式 13812345678 和 test@example.com")
	f.Add("身份证 11010519491231002X")
	f.Add("")
	f.Add("🙂‍↔️ combining \u0301 zero\u200bwidth")

	eng, err := NewRuleEngine(nil)
	if err != nil {
		f.Fatalf("NewRuleEngine(nil): %v", err)
	}
	pipe := NewPipeline(eng)

	f.Fuzz(func(t *testing.T, text string) {
		doc := NewDocument("fuzz", text)
		if _, err := pipe.Detect(context.Background(), doc); err != nil {
			t.Fatalf("Detect() error = %v", err)
		}
	})
}
