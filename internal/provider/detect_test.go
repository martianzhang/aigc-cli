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

func TestDetect_Agnes(t *testing.T) {
	cases := []string{
		"https://apihub.agnes-ai.com",
		"https://apihub.agnes-ai.com/v1",
		"https://apihub.agnes-ai.cn",
		"https://apihub.agnes-ai.cn/v1",
		"https://agnes-ai.com",
	}
	for _, url := range cases {
		if got := Detect(url); got != Agnes {
			t.Errorf("Detect(%q) = %v, want Agnes", url, got)
		}
	}
}

func TestDetect_Zeekai(t *testing.T) {
	tests := []struct {
		url  string
		want Type
	}{
		{"https://api.zeekai.cc", Zeekai},
		{"https://api.zeekai.cc/v1", Zeekai},
		{"https://zeekai.cc/v1", Zeekai},
		{"https://notzeekai.cc.evil.com", OpenAI},
		{"https://openrouter.ai/api/v1", OpenRouter},
	}
	for _, tc := range tests {
		if got := Detect(tc.url); got != tc.want {
			t.Errorf("Detect(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
	if !IsZeekai("https://api.zeekai.cc/v1") {
		t.Error("IsZeekai should be true for zeekai.cc")
	}
	if IsZeekai("https://openrouter.ai/api/v1") {
		t.Error("IsZeekai should be false for openrouter.ai")
	}
}

func TestDetect_Pollinations(t *testing.T) {
	tests := []struct {
		url  string
		want Type
	}{
		{"https://gen.pollinations.ai", Pollinations},
		{"https://gen.pollinations.ai/v1", Pollinations},
		{"https://text.pollinations.ai", Pollinations},
		{"https://image.pollinations.ai/v1", Pollinations},
		{"https://pollinations.ai.evil.com", OpenAI},
		{"https://openrouter.ai/api/v1", OpenRouter},
	}
	for _, tc := range tests {
		if got := Detect(tc.url); got != tc.want {
			t.Errorf("Detect(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
	if !IsPollinations("https://gen.pollinations.ai/v1") {
		t.Error("IsPollinations should be true for pollinations.ai")
	}
	if IsPollinations("https://api.apimart.ai") {
		t.Error("IsPollinations should be false for apimart.ai")
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

func TestDetect_IsAgnes(t *testing.T) {
	if !IsAgnes("https://apihub.agnes-ai.com/v1") {
		t.Error("IsAgnes should be true for agnes-ai.com")
	}
	if IsAgnes("https://openrouter.ai/api/v1") {
		t.Error("IsAgnes should be false for openrouter.ai")
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
	if ModelScope.String() != "ModelScope" {
		t.Errorf("ModelScope.String() = %q", ModelScope.String())
	}
	if Agnes.String() != "Agnes" {
		t.Errorf("Agnes.String() = %q", Agnes.String())
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
	if Agnes.IsAsync() {
		t.Error("Agnes should not be async")
	}
}

func TestDetect_Gemini(t *testing.T) {
	tests := []struct {
		url  string
		want Type
	}{
		{"https://generativelanguage.googleapis.com", Gemini},
		{"https://generativelanguage.googleapis.com/v1beta", Gemini},
		{"https://generativelanguage.googleapis.com/v1beta/models", Gemini},
		// The OpenAI-compatible /openai suffix is intentionally reported as
		// OpenAI so Gemini-OpenAI requests use the OpenAI-compatible path.
		{"https://generativelanguage.googleapis.com/v1beta/openai", OpenAI},
		{"https://generativelanguage.googleapis.com/v1beta/openai/", OpenAI},
		// gemini.google.com is not in the Gemini domain list, so it defaults to OpenAI.
		{"https://gemini.google.com", OpenAI},
		{"https://generativelanguage.googleapis.com.evil.com", OpenAI},
		{"https://openrouter.ai/api/v1", OpenRouter},
	}
	for _, tc := range tests {
		if got := Detect(tc.url); got != tc.want {
			t.Errorf("Detect(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestDetect_Bailian(t *testing.T) {
	tests := []struct {
		url  string
		want Type
	}{
		{"https://dashscope.aliyuncs.com", Bailian},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", Bailian},
		{"https://dashscope-intl.aliyuncs.com/compatible-mode/v1", Bailian},
		// Workspace-scoped native host, e.g. {WorkspaceId}.cn-beijing.maas.aliyuncs.com.
		{"https://ws-abc123.cn-beijing.maas.aliyuncs.com", Bailian},
		{"https://ws-abc123.cn-beijing.maas.aliyuncs.com/api/v1", Bailian},
		{"https://dashscope.aliyuncs.com.evil.com", OpenAI},
		{"https://dashscope.aliyuncs.com.evil.com/v1", OpenAI},
		{"https://openrouter.ai/api/v1", OpenRouter},
	}
	for _, tc := range tests {
		if got := Detect(tc.url); got != tc.want {
			t.Errorf("Detect(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestDetect_ModelScope(t *testing.T) {
	tests := []struct {
		url  string
		want Type
	}{
		{"https://api-inference.modelscope.cn", ModelScope},
		{"https://api-inference.modelscope.cn/v1", ModelScope},
		{"https://api-inference.modelscope.ai", ModelScope},
		{"https://api-inference.modelscope.ai/v1", ModelScope},
		// Only the api-inference.* hosts are recognized; the apex domain falls through.
		{"https://modelscope.cn", OpenAI},
		{"https://api-inference.modelscope.cn.evil.com", OpenAI},
		{"https://openrouter.ai/api/v1", OpenRouter},
	}
	for _, tc := range tests {
		if got := Detect(tc.url); got != tc.want {
			t.Errorf("Detect(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

// TestDetect_IsOpenAI covers the OpenAI/default branch. The package does not
// export an IsOpenAI helper, so the assertion goes through Detect directly.
func TestDetect_IsOpenAI(t *testing.T) {
	isOpenAI := func(baseURL string) bool { return Detect(baseURL) == OpenAI }
	tests := []struct {
		url  string
		want bool
	}{
		{"https://api.openai.com", true},
		{"https://api.openai.com/v1", true},
		// Unknown relays default to OpenAI-compatible.
		{"https://custom.relay.com/v1", true},
		{"https://my-gateway.internal", true},
		{"https://openrouter.ai/api/v1", false},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", false},
		// Empty URL is Unknown, not OpenAI.
		{"", false},
	}
	for _, tc := range tests {
		if got := isOpenAI(tc.url); got != tc.want {
			t.Errorf("isOpenAI(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestDetect_IsModelScope(t *testing.T) {
	if !IsModelScope("https://api-inference.modelscope.cn/v1") {
		t.Error("IsModelScope should be true for api-inference.modelscope.cn")
	}
	if !IsModelScope("https://api-inference.modelscope.ai") {
		t.Error("IsModelScope should be true for api-inference.modelscope.ai")
	}
	if IsModelScope("https://api.openai.com/v1") {
		t.Error("IsModelScope should be false for api.openai.com")
	}
	if IsModelScope("https://modelscope.cn") {
		t.Error("IsModelScope should be false for the modelscope.cn apex domain")
	}
}

func TestDetect_IsGemini(t *testing.T) {
	if !IsGemini("https://generativelanguage.googleapis.com/v1beta") {
		t.Error("IsGemini should be true for generativelanguage.googleapis.com")
	}
	// /openai reports as OpenAI, so IsGemini must be false there.
	if IsGemini("https://generativelanguage.googleapis.com/v1beta/openai") {
		t.Error("IsGemini should be false for the /openai variant")
	}
	if IsGemini("https://api.openai.com/v1") {
		t.Error("IsGemini should be false for api.openai.com")
	}
	// IsGeminiDomain is deliberately broader: it still recognizes the
	// OpenAI-compatible /openai endpoint as a Gemini host.
	if !IsGeminiDomain("https://generativelanguage.googleapis.com/v1beta/openai") {
		t.Error("IsGeminiDomain should be true for the /openai variant")
	}
	if IsGeminiDomain("https://api.openai.com/v1") {
		t.Error("IsGeminiDomain should be false for api.openai.com")
	}
}

func TestDetect_IsBailian(t *testing.T) {
	if !IsBailian("https://dashscope.aliyuncs.com/compatible-mode/v1") {
		t.Error("IsBailian should be true for dashscope.aliyuncs.com")
	}
	if !IsBailian("https://ws-abc123.cn-beijing.maas.aliyuncs.com") {
		t.Error("IsBailian should be true for maas.aliyuncs.com")
	}
	if IsBailian("https://api.openai.com/v1") {
		t.Error("IsBailian should be false for api.openai.com")
	}
	if IsBailian("https://dashscope.aliyuncs.com.evil.com") {
		t.Error("IsBailian should be false for a lookalike host")
	}
}
