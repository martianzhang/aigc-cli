package audio

import "github.com/martianzhang/aigc-cli/internal/service"

// printAudioResult prints the H3 footer for TTS, SFX and transcription files.
func printAudioResult(files []string, model, provider string) {
	service.PrintResultSummary(service.ResultSummary{
		Files:    files,
		Model:    model,
		Provider: provider,
	})
}
