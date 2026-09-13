package knowledge

import "testing"

func TestExtractHost(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"host only", "https://example.com/a/b", "example.com"},
		{"host with port", "https://example.com:8080/x", "example.com:8080"},
		{"invalid", "://bad", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractHost(tc.in); got != tc.want {
				t.Errorf("extractHost(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveSearchStrategy(t *testing.T) {
	tests := []struct {
		provider   string
		wantStrat  string
		wantManual []string
	}{
		{"auto", "auto", nil},
		{"", "auto", nil},
		{"free", "cheap", nil},
		{"cheap", "cheap", nil},
		{"quality", "quality", nil},
		{"firecrawl", "manual", []string{"firecrawl"}},
	}
	for _, tc := range tests {
		t.Run(tc.provider, func(t *testing.T) {
			strat, manual := resolveSearchStrategy(tc.provider)
			if strat != tc.wantStrat {
				t.Errorf("strategy = %q, want %q", strat, tc.wantStrat)
			}
			if len(manual) != len(tc.wantManual) {
				t.Fatalf("manual = %v, want %v", manual, tc.wantManual)
			}
			for i := range manual {
				if manual[i] != tc.wantManual[i] {
					t.Fatalf("manual = %v, want %v", manual, tc.wantManual)
				}
			}
		})
	}
}
