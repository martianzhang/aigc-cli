package provider

// Balance-host predicates.
//
// These providers expose a balance/quota endpoint but share the generic
// OpenAI-compatible generation protocol, so they are deliberately NOT added to
// Detect() (which drives generation routing). Balance lookup matches them by
// host instead, keeping the two concerns independent.
//
// SiliconFlow is intentionally absent: its only balance endpoint (/v1/user/info)
// was retired on 2026-08-14 with no replacement announced yet, so it falls
// through to the generic probe and reports "not available".

// deepseekDomains lists hosts where DeepSeek's API is served. Its balance
// endpoint (GET /user/balance) lives at the host root, outside the /v1 prefix.
var deepseekDomains = []string{"deepseek.com"}

// moonshotDomains lists hosts where Moonshot (Kimi) API is served.
var moonshotDomains = []string{"moonshot.cn", "moonshot.ai"}

// IsDeepSeek reports whether baseURL is a DeepSeek API host.
func IsDeepSeek(baseURL string) bool { return matchesAnyDomain(baseURL, deepseekDomains) }

// IsMoonshot reports whether baseURL is a Moonshot (Kimi) API host.
func IsMoonshot(baseURL string) bool { return matchesAnyDomain(baseURL, moonshotDomains) }

// matchesAnyDomain reports whether baseURL's host is, or is a subdomain of, any
// of domains.
func matchesAnyDomain(baseURL string, domains []string) bool {
	for _, d := range domains {
		if matchDomain(baseURL, d) {
			return true
		}
	}
	return false
}
