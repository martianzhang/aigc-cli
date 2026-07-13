package service

import (
	"encoding/json"
	"html"
	"strings"
)

// inferSynthID returns a SynthIDResult based on C2PA metadata heuristics.
// Google pairs SynthID with C2PA for Imagen/Gemini-generated images.
// OpenAI also uses invisible watermarks (possibly SynthID) for DALL-E images.
func inferSynthID(vendor, software, source string) *SynthIDResult {
	lowVendor := strings.ToLower(vendor)
	lowSoftware := strings.ToLower(software)
	lowSource := strings.ToLower(source)

	// Google products
	if strings.Contains(lowVendor, "google") ||
		strings.Contains(lowSoftware, "imagen") ||
		strings.Contains(lowSoftware, "gemini") ||
		strings.Contains(lowSoftware, "goo_") {
		return &SynthIDResult{
			Present:   true,
			Likely:    true,
			Source:    vendor,
			Inference: "C2PA manifest from Google — SynthID watermark likely embedded",
		}
	}

	// OpenAI products (DALL-E, GPT-Image)
	if strings.Contains(lowVendor, "openai") ||
		strings.Contains(lowSoftware, "dall") ||
		strings.Contains(lowSoftware, "gpt-image") {
		return &SynthIDResult{
			Present:   true,
			Likely:    true,
			Source:    vendor,
			Inference: "C2PA manifest from OpenAI — invisible watermark likely embedded (SynthID or similar)",
		}
	}

	// Generic AI-generated flag
	if strings.Contains(lowSource, "ai generated") || strings.Contains(lowSource, "algorithmic") {
		return &SynthIDResult{
			Present:   false,
			Likely:    true,
			Source:    source,
			Inference: "AI-generated content flagged in C2PA — invisible watermark may be present",
		}
	}

	return nil
}

// parseTC260Result parses TC260 data into a structured result.
func parseTC260Result(tc260data string) *TC260Result {
	result := &TC260Result{
		Present: true,
		Data:    tc260data,
		Fields:  make(map[string]string),
	}

	decoded := strings.TrimSpace(html.UnescapeString(tc260data))

	// Some providers wrap the JSON in a JSON string: "{\"Label":...}"
	if strings.HasPrefix(decoded, `"`) && strings.HasSuffix(decoded, `"`) {
		var inner string
		if json.Unmarshal([]byte(decoded), &inner) == nil {
			decoded = inner
		}
	}

	var parsed map[string]string
	if json.Unmarshal([]byte(decoded), &parsed) == nil {
		for k, v := range parsed {
			result.Fields[k] = v
		}
	} else {
		var nested map[string]map[string]string
		if json.Unmarshal([]byte(decoded), &nested) == nil {
			if aigc, ok := nested["AIGC"]; ok {
				for k, v := range aigc {
					result.Fields[k] = v
				}
			}
		}
	}

	if cp, ok := result.Fields[ContentProducerKey]; ok {
		result.Provider = resolveContentProducer(cp)
	}
	// Some providers only include ContentPropagator (same entity code).
	if result.Provider == "" {
		if cp, ok := result.Fields["ContentPropagator"]; ok {
			result.Provider = resolveContentProducer(cp)
		}
	}

	return result
}

// tc260ProviderCodes maps ContentProducer entity codes (chars 2-21 of the
// 27-char full code) to provider names. To discover a new code:
//
//	aigc-cli detect --verbose <ai-generated-image.png>
//
// then copy the ContentProducer value and share it to add here.
// References: GB 45438-2025, TC260 service provider encoding rules.
var tc260ProviderCodes = map[string]string{
	"1191110102MACQD9K640": "字节跳动 (ByteDance) — 豆包(doubao) / 火山引擎",
	"119144030008867405X2": "字节跳动 (ByteDance) — 即梦(jimeng)",
	"1191110108MA01KP2T5U": "智谱AI (Zhipu) — 清言 / GLM",
	"1191110000802100433B": "百度 (Baidu) — 文心一言",
	"119144030071526726XG": "腾讯 (Tencent) — 混元",
	"1191330106MA2CFLDG4R": "阿里巴巴 (Alibaba) — 通义千问 / 通义万相",
	"1191440101MA9Y9T4H7A": "阿里巴巴 (Alibaba) — 通义千问(qwen)",
	// ── 以下为常见大模型厂商（需要实际图片确认编码） ──
	// To add: generate an image with the provider's tool, run
	//   aigc-cli detect --verbose <image>
	// and submit the ContentProducer code as a PR.
	// "????????????????????": "DeepSeek — 深度求索",
	// "????????????????????": "百度 (Baidu) — 文心一言",
	// "????????????????????": "阿里巴巴 (Alibaba) — 通义千问 / 通义万相",
	// "????????????????????": "腾讯 (Tencent) — 混元",
	// "????????????????????": "小米 (Xiaomi) — MiMo",
	// "????????????????????": "美团 (Meituan) — 美团AI",
	// "????????????????????": "科大讯飞 (iFlytek) — 星火",
	// "????????????????????": "月之暗面 (Moonshot) — Kimi",
	// "????????????????????": "百川智能 (Baichuan) — 百川",
	// "????????????????????": "MiniMax — 海螺AI",
	// "????????????????????": "商汤科技 (SenseTime) — 日日新",
	// "????????????????????": "昆仑万维 (Kunlun) — 天工AI",
	// "????????????????????": "华为 (Huawei) — 盘古",
}

// resolveContentProducer looks up the entity code portion of a TC260
// ContentProducer code and returns a recognizable provider name.
func resolveContentProducer(code string) string {
	// Full code is 27 chars: version(2) + entity(20) + service(5)
	// Try matching from longest prefix
	if len(code) >= 22 {
		entityCode := code[2:22] // extract the entity code (20 chars)
		if name, ok := tc260ProviderCodes[entityCode]; ok {
			return name
		}
	}
	// Fallback: try exact match
	if name, ok := tc260ProviderCodes[code]; ok {
		return name
	}
	return ""
}
