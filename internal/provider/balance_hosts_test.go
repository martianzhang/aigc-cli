package provider

import "testing"

func TestBalanceHostPredicates(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		ds, ms bool
	}{
		{"deepseek", "https://api.deepseek.com/v1", true, false},
		{"moonshot cn", "https://api.moonshot.cn/v1", false, true},
		{"moonshot ai", "https://api.moonshot.ai/v1", false, true},
		{"openrouter", "https://openrouter.ai/api/v1", false, false},
		{"siliconflow", "https://api.siliconflow.cn/v1", false, false},
		{"apimart", "https://api.apimart.ai/v1", false, false},
		{"empty", "", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsDeepSeek(tc.url); got != tc.ds {
				t.Errorf("IsDeepSeek(%q) = %v, want %v", tc.url, got, tc.ds)
			}
			if got := IsMoonshot(tc.url); got != tc.ms {
				t.Errorf("IsMoonshot(%q) = %v, want %v", tc.url, got, tc.ms)
			}
		})
	}
}
