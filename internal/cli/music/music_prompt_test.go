package music

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestBuildMusicGenerateReqReadsPromptFile(t *testing.T) {
	oldPrompt, oldModel, oldStyle, oldTitle, oldLyrics, oldFormat := musicPrompt, musicModel, musicStyle, musicTitle, musicLyrics, musicFormat
	oldInstrumental, oldDuration := musicInstrumental, musicDuration
	oldJSON, oldInject := musicJSONInput, musicInjectJSON
	t.Cleanup(func() {
		musicPrompt, musicModel, musicStyle, musicTitle, musicLyrics, musicFormat = oldPrompt, oldModel, oldStyle, oldTitle, oldLyrics, oldFormat
		musicInstrumental, musicDuration = oldInstrumental, oldDuration
		musicJSONInput, musicInjectJSON = oldJSON, oldInject
	})

	musicModel, musicStyle, musicTitle, musicLyrics, musicFormat = "suno", "", "", "", ""
	musicInstrumental, musicDuration = false, 0
	musicJSONInput, musicInjectJSON = "", ""

	path := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(path, []byte("city pop"), 0o600); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
	musicPrompt = path

	req, err := buildMusicGenerateReq(&cobra.Command{})
	if err != nil {
		t.Fatalf("buildMusicGenerateReq() error = %v", err)
	}
	if req.Prompt != "city pop" {
		t.Errorf("Prompt = %q, want the file contents", req.Prompt)
	}
}

func TestBuildMusicGenerateReqKeepsLiteralPrompt(t *testing.T) {
	oldPrompt, oldModel := musicPrompt, musicModel
	oldJSON, oldInject := musicJSONInput, musicInjectJSON
	t.Cleanup(func() {
		musicPrompt, musicModel = oldPrompt, oldModel
		musicJSONInput, musicInjectJSON = oldJSON, oldInject
	})

	musicModel, musicJSONInput, musicInjectJSON = "suno", "", ""
	musicPrompt = "city pop"

	req, err := buildMusicGenerateReq(&cobra.Command{})
	if err != nil {
		t.Fatalf("buildMusicGenerateReq() error = %v", err)
	}
	if req.Prompt != "city pop" {
		t.Errorf("Prompt = %q, want the literal value", req.Prompt)
	}
}
