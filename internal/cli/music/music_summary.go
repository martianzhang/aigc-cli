package music

import (
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// printMusicResult prints the H3 footer. Only APIMart exposes a task-query
// command, so only that backend gets a task Next hint.
func printMusicResult(req *types.MusicGenerateRequest, providerType provider.Type, saved []string, taskID string) {
	model := ""
	if req != nil {
		model = req.Model
	}
	next := ""
	if providerType == provider.APIMart && taskID != "" {
		next = "aigc-cli task " + taskID
	}
	service.PrintResultSummary(service.ResultSummary{
		Files:    saved,
		Model:    model,
		Provider: providerType.String(),
		TaskID:   taskID,
		Next:     next,
	})
}
