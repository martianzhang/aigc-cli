package audio

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestDetectAudioFormat(t *testing.T) {
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

func TestIsLocalAudioMode(t *testing.T) {
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
