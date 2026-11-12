package video

import (
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// savePromptFile saves the generation prompt to video_{taskID}.md when
// --save-prompt is set.
func savePromptFile(taskID, prompt string) {
	if !options.Shared.SavePrompt {
		return
	}
	if err := service.SavePrompt(options.Shared.OutputDir, "video", taskID, prompt); err != nil {
		ui.Eprintf("Warning: %v\n", err)
	}
}
