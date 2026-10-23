package types

import "testing"

func TestAudioDefaultsSpeakResolution(t *testing.T) {
	tests := []struct {
		name       string
		defaults   *AudioDefaults
		wantModel  string
		wantVoice  string
		wantFormat string
		wantProv   string
	}{
		{
			name:     "nil",
			defaults: nil,
		},
		{
			name:       "nested section",
			defaults:   &AudioDefaults{Speak: &AudioSpeakDefaults{Provider: "nested", Model: "m1", Voice: "v1", Format: "wav"}},
			wantModel:  "m1",
			wantVoice:  "v1",
			wantFormat: "wav",
			wantProv:   "nested",
		},
		{
			name:     "empty section",
			defaults: &AudioDefaults{Speak: &AudioSpeakDefaults{}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.defaults.SpeakModel(); got != tc.wantModel {
				t.Errorf("SpeakModel() = %q, want %q", got, tc.wantModel)
			}
			if got := tc.defaults.SpeakVoice(); got != tc.wantVoice {
				t.Errorf("SpeakVoice() = %q, want %q", got, tc.wantVoice)
			}
			if got := tc.defaults.SpeakFormat(); got != tc.wantFormat {
				t.Errorf("SpeakFormat() = %q, want %q", got, tc.wantFormat)
			}
			if got := tc.defaults.SpeakProvider(); got != tc.wantProv {
				t.Errorf("SpeakProvider() = %q, want %q", got, tc.wantProv)
			}
		})
	}
}

func TestAudioDefaultsTranscribeAndSFXResolution(t *testing.T) {
	d := &AudioDefaults{
		Transcribe: &AudioTranscribeDefaults{Provider: "asr", Model: "whisper-large-v3", Language: "zh"},
		SFX:        &AudioSFXDefaults{Provider: "elevenlabs", Model: "eleven_text_to_sound_v2", Format: "mp3"},
	}

	if got := d.TranscribeModel(); got != "whisper-large-v3" {
		t.Errorf("TranscribeModel() = %q, want whisper-large-v3", got)
	}
	if got := d.TranscribeProvider(); got != "asr" {
		t.Errorf("TranscribeProvider() = %q, want asr", got)
	}
	if got := d.TranscribeLanguage(); got != "zh" {
		t.Errorf("TranscribeLanguage() = %q, want zh", got)
	}
	if got := d.SFXProvider(); got != "elevenlabs" {
		t.Errorf("SFXProvider() = %q, want elevenlabs", got)
	}
	if got := d.SFXModel(); got != "eleven_text_to_sound_v2" {
		t.Errorf("SFXModel() = %q, want eleven_text_to_sound_v2", got)
	}
	if got := d.SFXFormat(); got != "mp3" {
		t.Errorf("SFXFormat() = %q, want mp3", got)
	}

	var nilDefaults *AudioDefaults
	if nilDefaults.TranscribeModel() != "" || nilDefaults.SFXModel() != "" || nilDefaults.SpeakProvider() != "" {
		t.Error("nil AudioDefaults accessors must return empty strings")
	}
}
