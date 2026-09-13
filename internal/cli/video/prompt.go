package video

import (
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// savePromptFile saves the generation prompt to image_{taskID}.md when --save-prompt is set.
func savePromptFile(taskID, prompt string) {
	if !options.Shared.SavePrompt {
		return
	}
	service.SavePrompt(options.Shared.OutputDir, taskID, prompt)
}
