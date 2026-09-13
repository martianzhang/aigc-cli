package cmd

import "testing"

func TestMaskBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"host only", "https://api.apimart.ai", "api.apimart.ai"},
		{"host with port", "https://api.apimart.ai:8080/v1", "api.apimart.ai:8080"},
		{"invalid passthrough", "://bad", "://bad"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskBaseURL(tc.in); got != tc.want {
				t.Errorf("maskBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
