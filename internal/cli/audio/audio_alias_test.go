package audio

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestAudioSpeakPromptAlias(t *testing.T) {
	oldModel, oldInput, oldVoice := audioSpeechModel, audioSpeechInput, audioSpeechVoice
	oldFormat, oldSpeed := audioSpeechFormat, audioSpeechSpeed
	oldInstructions, oldPlay := audioSpeechInstructions, audioSpeechPlay
	t.Cleanup(func() {
		audioSpeechModel, audioSpeechInput, audioSpeechVoice = oldModel, oldInput, oldVoice
		audioSpeechFormat, audioSpeechSpeed = oldFormat, oldSpeed
		audioSpeechInstructions, audioSpeechPlay = oldInstructions, oldPlay
	})

	cmd := &cobra.Command{Use: "speak"}
	registerAudioSpeakFlags(cmd)

	flag := cmd.Flags().Lookup("prompt")
	if flag == nil {
		t.Fatal("--prompt flag is not registered")
	}
	if flag.Shorthand != "p" {
		t.Errorf("--prompt shorthand = %q, want %q", flag.Shorthand, "p")
	}
	if err := cmd.Flags().Set("prompt", "hello"); err != nil {
		t.Fatalf("set --prompt: %v", err)
	}
	if audioSpeechInput != "hello" {
		t.Errorf("audioSpeechInput = %q, want %q", audioSpeechInput, "hello")
	}
}

func TestAudioSFXPromptAlias(t *testing.T) {
	cmd := withSFXFlags(t)

	flag := cmd.Flags().Lookup("prompt")
	if flag == nil {
		t.Fatal("--prompt flag is not registered")
	}
	if flag.Shorthand != "p" {
		t.Errorf("--prompt shorthand = %q, want %q", flag.Shorthand, "p")
	}
	if err := cmd.Flags().Set("prompt", "whoosh"); err != nil {
		t.Fatalf("set --prompt: %v", err)
	}
	if sfxInput != "whoosh" {
		t.Errorf("sfxInput = %q, want %q", sfxInput, "whoosh")
	}
}
