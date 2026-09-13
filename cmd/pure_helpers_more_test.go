package cmd

import (
	"image/color"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestCmdParseHexColor(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    color.RGBA
		wantErr bool
	}{
		{"with hash", "#ff0000", color.RGBA{R: 255, G: 0, B: 0, A: 255}, false},
		{"without hash", "ff0000", color.RGBA{R: 255, G: 0, B: 0, A: 255}, false},
		{"mixed", "#A1b2C3", color.RGBA{R: 0xA1, G: 0xB2, B: 0xC3, A: 255}, false},
		{"white", "#ffffff", color.RGBA{R: 255, G: 255, B: 255, A: 255}, false},
		{"too short", "#f00", color.RGBA{}, true},
		{"not hex", "#zzzzzz", color.RGBA{}, true},
		{"empty", "", color.RGBA{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseHexColor(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseHexColor(%q) expected error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseHexColor(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("parseHexColor(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCmdParseOffset(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		dx, dy, err := parseOffset("10,-20")
		if err != nil || dx != 10 || dy != -20 {
			t.Fatalf("got (%d,%d,%v), want (10,-20,nil)", dx, dy, err)
		}
	})

	for _, bad := range []string{"10", "10,20,30", "a,b"} {
		t.Run("invalid "+bad, func(t *testing.T) {
			if _, _, err := parseOffset(bad); err == nil {
				t.Fatalf("parseOffset(%q) expected error", bad)
			}
		})
	}
}

func TestCmdIsHTML(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want bool
	}{
		{"html tag", []byte("<html>"), true},
		{"leading whitespace", []byte("  \n\t <div>"), true},
		{"json", []byte(`{"a":1}`), false},
		{"empty", []byte(""), false},
		{"only whitespace", []byte("   \n"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHTML(tc.in); got != tc.want {
				t.Errorf("isHTML(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCmdDetectAudioFormat(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"a.mp3", "mp3"},
		{"a.wav", "wav"},
		{"dir/a.flac", "flac"},
		{"a.m4a", "m4a"},
		{"a.ogg", "ogg"},
		{"a.webm", "webm"},
		{"a.aac", "aac"},
		{"a.FLAC", "wav"},
		{"a.xyz", "wav"},
		{"noext", "wav"},
		{"a.b.mp3", "mp3"},
	}
	for _, tc := range tests {
		if got := detectAudioFormat(tc.in); got != tc.want {
			t.Errorf("detectAudioFormat(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCmdIsLocalAudioMode(t *testing.T) {
	tests := []struct {
		name  string
		p     *provider.EffectiveProvider
		local bool
		want  bool
	}{
		{"nil provider", nil, false, true},
		{"local type", &provider.EffectiveProvider{Type: types.ProviderLocal}, false, true},
		{"empty name and model", &provider.EffectiveProvider{}, false, true},
		{"online provider", &provider.EffectiveProvider{Name: "openai", Model: "gpt-4o"}, false, false},
		{"local flag forced", &provider.EffectiveProvider{Name: "openai", Model: "gpt-4o"}, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLocalAudioMode(tc.p, tc.local); got != tc.want {
				t.Errorf("isLocalAudioMode(%+v, %v) = %v, want %v", tc.p, tc.local, got, tc.want)
			}
		})
	}
}

func TestCmdResolveSearchStrategy(t *testing.T) {
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
