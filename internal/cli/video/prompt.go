package video

import (
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// savePromptFile saves the generation prompt to video_{taskID}.md when
// --save-prompt is set.
func savePromptFile(taskID, prompt string) {
	if !options.Shared.SavePrompt {
		return
	}
	if err := service.SavePrompt(options.Shared.OutputDir, "video", taskID, prompt); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
}
