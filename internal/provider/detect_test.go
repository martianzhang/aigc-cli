package provider

import "testing"

func TestDetect_APIMart(t *testing.T) {
	cases := []string{
		"https://api.apimart.ai",
		"https://api.apimart.ai/v1",
		"https://apib.ai",
		"https://api.aiuxu.com",
		"https://api.aishuch.com",
	}
	for _, url := range cases {
		if got := Detect(url); got != APIMart {
			t.Errorf("Detect(%q) = %v, want APIMart", url, got)
		}
	}
}

func TestDetect_OpenRouter(t *testing.T) {
	cases := []string{
		"https://openrouter.ai/api/v1",
		"https://openrouter.ai",
	}
	for _, url := range cases {
		if got := Detect(url); got != OpenRouter {
			t.Errorf("Detect(%q) = %v, want OpenRouter", url, got)
		}
	}
}

func TestDetect_Yunwu(t *testing.T) {
	cases := []string{
		"https://api.yunwu.ai",
		"https://yunwu.ai",
	}
	for _, url := range cases {
		if got := Detect(url); got != Yunwu {
			t.Errorf("Detect(%q) = %v, want Yunwu", url, got)
		}
	}
}

func TestDetect_OpenAI(t *testing.T) {
	cases := []string{
		"https://api.openai.com/v1",
		"https://api.openai.com",
	}
	for _, url := range cases {
		if got := Detect(url); got != OpenAI {
			t.Errorf("Detect(%q) = %v, want OpenAI", url, got)
		}
	}
}

func TestDetect_Unknown(t *testing.T) {
	if got := Detect(""); got != Unknown {
		t.Errorf("Detect(\"\") = %v, want Unknown", got)
	}
	if got := Detect("https://custom.relay.com/v1"); got != OpenAI {
		t.Errorf("Detect(custom) = %v, want OpenAI (default)", got)
	}
}

func TestDetect_IsYunwu(t *testing.T) {
	if !IsYunwu("https://api.yunwu.ai/v1") {
		t.Error("IsYunwu should be true for yunwu.ai")
	}
	if IsYunwu("https://api.apimart.ai") {
		t.Error("IsYunwu should be false for apimart.ai")
	}
}

func TestDetect_IsAPIMart(t *testing.T) {
	if !IsAPIMart("https://api.apimart.ai/v1") {
		t.Error("IsAPIMart should be true for apimart.ai")
	}
	if IsAPIMart("https://openrouter.ai/api/v1") {
		t.Error("IsAPIMart should be false for openrouter.ai")
	}
}

func TestDetect_IsOpenRouter(t *testing.T) {
	if !IsOpenRouter("https://openrouter.ai/api/v1") {
		t.Error("IsOpenRouter should be true for openrouter.ai")
	}
	if IsOpenRouter("https://api.apimart.ai") {
		t.Error("IsOpenRouter should be false for apimart.ai")
	}
}

func TestType_String(t *testing.T) {
	if APIMart.String() != "APIMart" {
		t.Errorf("APIMart.String() = %q", APIMart.String())
	}
	if OpenRouter.String() != "OpenRouter" {
		t.Errorf("OpenRouter.String() = %q", OpenRouter.String())
	}
	if OpenAI.String() != "OpenAI" {
		t.Errorf("OpenAI.String() = %q", OpenAI.String())
	}
	if Yunwu.String() != "Yunwu（云雾AI）" {
		t.Errorf("Yunwu.String() = %q", Yunwu.String())
	}
	if Unknown.String() != "unknown" {
		t.Errorf("Unknown.String() = %q", Unknown.String())
	}
}

func TestIsLocalEndpoint(t *testing.T) {
	cases := []struct {
		url      string
		expected bool
	}{
		{"http://localhost:11434", true},
		{"http://localhost:11434/v1", true},
		{"http://127.0.0.1:11434", true},
		{"http://127.0.0.1:11434/v1", true},
		{"http://::1:11434", true},
		{"https://api.openai.com/v1", false},
		{"https://openrouter.ai/api/v1", false},
		{"https://api.apimart.ai", false},
		{"http://192.168.1.100:11434", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsLocalEndpoint(c.url); got != c.expected {
			t.Errorf("IsLocalEndpoint(%q) = %v, want %v", c.url, got, c.expected)
		}
	}
}

func TestType_IsAsync(t *testing.T) {
	if !APIMart.IsAsync() {
		t.Error("APIMart should be async")
	}
	if OpenAI.IsAsync() {
		t.Error("OpenAI should not be async")
	}
	if OpenRouter.IsAsync() {
		t.Error("OpenRouter should not be async")
	}
}
