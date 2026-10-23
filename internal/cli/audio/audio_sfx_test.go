package audio

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// withSFXFlags resets the package-level SFX flags around a test.
func withSFXFlags(t *testing.T) *cobra.Command {
	t.Helper()
	oldInput, oldDuration, oldLoop, oldInfluence := sfxInput, sfxDuration, sfxLoop, sfxInfluence
	oldModel, oldFormat, oldPlay := sfxModel, sfxFormat, sfxPlay
	oldLocal, oldSeed, oldNoNorm := sfxLocal, sfxSeed, sfxNoNormalize
	t.Cleanup(func() {
		sfxInput, sfxDuration, sfxLoop, sfxInfluence = oldInput, oldDuration, oldLoop, oldInfluence
		sfxModel, sfxFormat, sfxPlay = oldModel, oldFormat, oldPlay
		sfxLocal, sfxSeed, sfxNoNormalize = oldLocal, oldSeed, oldNoNorm
	})

	sfxInput, sfxDuration, sfxLoop, sfxInfluence = "", 0, false, 0.3
	sfxModel, sfxFormat, sfxPlay = "", "", false
	sfxLocal, sfxSeed, sfxNoNormalize = false, 0, false

	cmd := &cobra.Command{Use: "sfx"}
	registerAudioSFXFlags(cmd)
	return cmd
}

// withSFXConfig swaps in a config for the duration of one test.
func withSFXConfig(t *testing.T, cfg *types.Config) {
	t.Helper()
	old := options.Shared
	options.Shared = &options.SharedConfig{Cfg: cfg}
	t.Cleanup(func() { options.Shared = old })
}

func TestBuildSFXRequestElevenLabsDefaults(t *testing.T) {
	cmd := withSFXFlags(t)
	withSFXConfig(t, nil)

	p := &provider.EffectiveProvider{ProviderType: provider.ElevenLabs}
	req, err := buildSFXRequest(cmd, "door slam", p)
	if err != nil {
		t.Fatalf("buildSFXRequest() error = %v", err)
	}
	if req.Model != provider.DefaultSFXModelElevenLabs {
		t.Errorf("model = %q, want %s", req.Model, provider.DefaultSFXModelElevenLabs)
	}
	if req.DurationSeconds != 0 {
		t.Errorf("duration = %v, want 0 (auto)", req.DurationSeconds)
	}
	if req.PromptInfluence != nil {
		t.Errorf("influence = %v, want nil unless --influence is set", *req.PromptInfluence)
	}
}

func TestBuildSFXRequestElevenLabsFlagInfluence(t *testing.T) {
	cmd := withSFXFlags(t)
	withSFXConfig(t, nil)
	if err := cmd.Flags().Set("influence", "0"); err != nil {
		t.Fatal(err)
	}

	p := &provider.EffectiveProvider{ProviderType: provider.ElevenLabs}
	req, err := buildSFXRequest(cmd, "whoosh", p)
	if err != nil {
		t.Fatalf("buildSFXRequest() error = %v", err)
	}
	if req.PromptInfluence == nil || *req.PromptInfluence != 0 {
		t.Fatalf("influence = %v, want explicit 0", req.PromptInfluence)
	}
}

func TestBuildSFXRequestValidation(t *testing.T) {
	tests := []struct {
		name     string
		duration float64
		format   string
		loop     bool
		engine   provider.Type
		wantErr  bool
	}{
		{name: "elevenlabs auto duration", duration: 0, engine: provider.ElevenLabs},
		{name: "elevenlabs min duration", duration: 0.5, engine: provider.ElevenLabs},
		{name: "elevenlabs too short", duration: 0.1, engine: provider.ElevenLabs, wantErr: true},
		{name: "elevenlabs too long", duration: 31, engine: provider.ElevenLabs, wantErr: true},
		{name: "elevenlabs loop over 22s", duration: 23, loop: true, engine: provider.ElevenLabs, wantErr: true},
		{name: "stability zero becomes five", duration: 0, engine: provider.Stability},
		{name: "stability min duration", duration: 1, engine: provider.Stability},
		{name: "stability too long", duration: 191, engine: provider.Stability, wantErr: true},
		{name: "stability loop unsupported", duration: 5, loop: true, engine: provider.Stability, wantErr: true},
		{name: "bad format", duration: 2, format: "flac", engine: provider.ElevenLabs, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := withSFXFlags(t)
			withSFXConfig(t, nil)
			sfxDuration = tc.duration
			sfxFormat = tc.format
			sfxLoop = tc.loop

			p := &provider.EffectiveProvider{ProviderType: tc.engine}
			req, err := buildSFXRequest(cmd, "sound", p)
			if tc.wantErr {
				if err == nil {
					t.Fatal("buildSFXRequest() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildSFXRequest() error = %v", err)
			}
			if tc.engine == provider.Stability && tc.duration == 0 && req.DurationSeconds != 5 {
				t.Errorf("stability duration = %v, want 5", req.DurationSeconds)
			}
		})
	}
}

func TestBuildSFXRequestConfigDefaults(t *testing.T) {
	cmd := withSFXFlags(t)
	withSFXConfig(t, &types.Config{Defaults: &types.ConfigDefaults{
		Audio: &types.AudioDefaults{SFX: &types.AudioSFXDefaults{Model: "eleven_text_to_sound_v3", Format: "wav"}},
	}})

	p := &provider.EffectiveProvider{ProviderType: provider.ElevenLabs}
	req, err := buildSFXRequest(cmd, "sound", p)
	if err != nil {
		t.Fatalf("buildSFXRequest() error = %v", err)
	}
	if req.Model != "eleven_text_to_sound_v3" {
		t.Errorf("model = %q, want config default", req.Model)
	}
	if req.Format != "wav" {
		t.Errorf("format = %q, want wav", req.Format)
	}
}

func TestResolveSFXProviderAliases(t *testing.T) {
	tests := []struct {
		name    string
		envKey  string
		wantKey string
		baseURL string
		engine  provider.Type
	}{
		{name: "elevenlabs", envKey: "ELEVENLABS_API_KEY", wantKey: "xi-env", baseURL: provider.ElevenLabsBaseURL, engine: provider.ElevenLabs},
		{name: "stability", envKey: "STABILITY_API_KEY", wantKey: "sk-env", baseURL: provider.StabilityBaseURL, engine: provider.Stability},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withSFXConfig(t, &types.Config{Defaults: &types.ConfigDefaults{
				Audio: &types.AudioDefaults{SFX: &types.AudioSFXDefaults{Provider: tc.name}},
			}})
			t.Setenv(tc.envKey, tc.wantKey)

			p := resolveSFXProvider()
			if p == nil {
				t.Fatal("resolveSFXProvider() = nil")
			}
			if p.ProviderType != tc.engine {
				t.Errorf("provider type = %v, want %v", p.ProviderType, tc.engine)
			}
			if p.BaseURL != tc.baseURL {
				t.Errorf("base URL = %q, want %q", p.BaseURL, tc.baseURL)
			}
			if p.APIKey != tc.wantKey {
				t.Errorf("api key = %q, want %q", p.APIKey, tc.wantKey)
			}
		})
	}
}

func TestResolveSFXProviderKeepsCustomBaseURL(t *testing.T) {
	withSFXConfig(t, &types.Config{
		Providers: map[string]*types.NamedProvider{
			"elevenlabs": {BaseURL: "https://proxy.example.com", APIKey: "xi-proxy"},
		},
		Defaults: &types.ConfigDefaults{Audio: &types.AudioDefaults{SFX: &types.AudioSFXDefaults{Provider: "elevenlabs"}}},
	})

	p := resolveSFXProvider()
	if p == nil {
		t.Fatal("resolveSFXProvider() = nil")
	}
	if p.BaseURL != "https://proxy.example.com" {
		t.Errorf("base URL = %q, want the configured proxy", p.BaseURL)
	}
	if p.APIKey != "xi-proxy" {
		t.Errorf("api key = %q, want xi-proxy", p.APIKey)
	}
}

func TestCheckLocalSFXFlags(t *testing.T) {
	cmd := withSFXFlags(t)

	if err := checkLocalSFXFlags(cmd); err != nil {
		t.Fatalf("checkLocalSFXFlags() error = %v", err)
	}

	sfxLoop = true
	if err := checkLocalSFXFlags(cmd); err == nil {
		t.Error("--loop must be rejected in local mode")
	}
	sfxLoop = false

	if err := cmd.Flags().Set("influence", "0.5"); err != nil {
		t.Fatal(err)
	}
	if err := checkLocalSFXFlags(cmd); err == nil {
		t.Error("--influence must be rejected in local mode")
	}

	cmd = withSFXFlags(t)
	sfxFormat = "mp3"
	if err := checkLocalSFXFlags(cmd); err == nil {
		t.Error("--format mp3 must be rejected in local mode")
	}
	sfxFormat = "wav"
	if err := checkLocalSFXFlags(cmd); err != nil {
		t.Errorf("--format wav must be accepted, got %v", err)
	}
}

func TestCheckOnlineSFXFlags(t *testing.T) {
	cmd := withSFXFlags(t)
	if err := checkOnlineSFXFlags(cmd); err != nil {
		t.Fatalf("checkOnlineSFXFlags() error = %v", err)
	}
	if err := cmd.Flags().Set("seed", "42"); err != nil {
		t.Fatal(err)
	}
	if err := checkOnlineSFXFlags(cmd); err == nil {
		t.Error("--seed must be rejected in online mode")
	}

	cmd = withSFXFlags(t)
	if err := cmd.Flags().Set("no-normalize", "true"); err != nil {
		t.Fatal(err)
	}
	if err := checkOnlineSFXFlags(cmd); err == nil {
		t.Error("--no-normalize must be rejected in online mode")
	}
}
