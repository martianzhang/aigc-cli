package mcp

import (
	"strings"
	"testing"
)

func TestValidateSpeechFormat(t *testing.T) {
	for _, format := range []string{"mp3", "wav", "opus", "aac", "flac", "pcm", "MP3"} {
		if err := validateSpeechFormat(format); err != nil {
			t.Errorf("validateSpeechFormat(%q) = %v, want nil", format, err)
		}
	}

	for _, format := range []string{"x/../../../pwned", "../evil", "", "ogg", "mp3/../../x", ".."} {
		err := validateSpeechFormat(format)
		if err == nil {
			t.Errorf("validateSpeechFormat(%q) = nil, want error", format)
			continue
		}
		if !strings.Contains(err.Error(), "invalid format") {
			t.Errorf("validateSpeechFormat(%q) error = %v", format, err)
		}
	}
}

func TestGenerateSpeechTool_formatEnum(t *testing.T) {
	tool := newGenerateSpeechTool("desc")

	prop, ok := tool.InputSchema.Properties["format"].(map[string]any)
	if !ok {
		t.Fatal("generate_speech schema missing format property")
	}
	enum, ok := prop["enum"].([]string)
	if !ok {
		t.Fatalf("format has no enum, got %#v", prop["enum"])
	}
	for _, want := range []string{"mp3", "wav", "opus", "aac", "flac", "pcm"} {
		found := false
		for _, got := range enum {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("format enum missing %q (enum: %v)", want, enum)
		}
	}
}
