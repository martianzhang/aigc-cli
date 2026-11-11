package redact

import (
	"context"
	"fmt"
	"strings"
)

// Engine detects spans in a document.
type Engine interface {
	Name() string
	Detect(ctx context.Context, doc *Document) (Result, error)
}

// Pipeline combines the rule layer with optional model engines.
type Pipeline struct {
	rules  *RuleEngine
	models []Engine
}

// NewPipeline builds a detection pipeline.
func NewPipeline(rules *RuleEngine, models ...Engine) *Pipeline {
	return &Pipeline{rules: rules, models: models}
}

// Detect runs rules first, then models, and resolves overlaps. Review spans are
// returned separately and are never applied automatically.
func (p *Pipeline) Detect(ctx context.Context, doc *Document) (Result, error) {
	var result Result
	if p.rules != nil {
		r := p.rules.Detect(doc)
		result.Spans = append(result.Spans, r.Spans...)
		result.Review = append(result.Review, r.Review...)
	}
	for _, eng := range p.models {
		r, err := eng.Detect(ctx, doc)
		if err != nil {
			return Result{}, err
		}
		for _, s := range r.Spans {
			if !validModelSpan(s) {
				continue
			}
			result.Spans = append(result.Spans, s)
		}
		result.Review = append(result.Review, r.Review...)
	}
	result.Spans = propagateExactSpans(doc, MergeSpans(result.Spans))
	result.Review = dedupeSpans(result.Review)
	return result, nil
}

func validModelSpan(s Span) bool {
	if s.Start < 0 || s.End <= s.Start {
		return false
	}
	switch s.Type {
	case "ID_CARD":
		return ValidateIDCard(s.Text)
	case "USCC":
		return ValidateUSCC(s.Text)
	case "BANK_CARD":
		return ValidateLuhn(s.Text)
	case "PHONE":
		return ValidatePhone(s.Text)
	default:
		return true
	}
}

func dedupeSpans(in []Span) []Span {
	seen := map[string]bool{}
	out := make([]Span, 0, len(in))
	for _, s := range in {
		key := fmt.Sprintf("%d:%d:%s", s.Start, s.End, s.Type)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, s)
	}
	return out
}

// CanonicalType maps model-specific labels to the engine's type names.
func CanonicalType(label string) string {
	up := strings.ToUpper(strings.TrimSpace(label))
	contains := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(up, strings.ToUpper(p)) {
				return true
			}
		}
		return false
	}
	switch {
	case strings.EqualFold(up, "AGE") || contains("年龄"):
		return "AGE"
	case contains("身份证", "ID_CARD", "NATIONAL_ID", "GOVERNMENT_ID", "ID_NUMBER"):
		return "ID_CARD"
	case contains("统一社会信用代码", "USCC", "TAX_ID", "CREDIT_CODE"):
		return "USCC"
	case contains("银行卡", "银行账号", "BANK_ACCOUNT", "BANK", "CREDIT_CARD", "CARD_NUMBER"):
		return "BANK_CARD"
	case contains("邮箱", "EMAIL"):
		return "EMAIL"
	case contains("电话", "手机", "PHONE", "TELEPHONE", "MOBILE"):
		return "PHONE"
	case contains("案号", "CASE_NO", "CASE_NUMBER"):
		return "CASE_NO"
	case contains("车牌", "PLATE", "VEHICLE_ID", "LICENSE_PLATE"):
		return "PLATE"
	case contains("护照", "PASSPORT"):
		return "PASSPORT"
	case contains("律师", "LAWYER"):
		return "LAWYER_NO"
	case contains("SSN", "SOCIAL_SECURITY"):
		return "SSN"
	case contains("机构", "组织", "公司", "单位", "ORG", "ORGANIZATION", "COMPANY"):
		return "ORG"
	case contains("地址", "地点", "住址", "ADDRESS", "LOCATION", "STREET", "GPE", "LOC", "FAC"):
		return "ADDRESS"
	case contains("人名", "姓名", "人物", "PERSON", "PER", "NAME"):
		return "PERSON"
	case contains("WORK_OF_ART", "WORK"):
		return "WORK"
	case contains("金额", "AMOUNT", "MONEY"):
		return "AMOUNT"
	case contains("日期", "时间", "DATE", "TIME"):
		return "DATE"
	default:
		return strings.ToUpper(strings.TrimSpace(label))
	}
}
