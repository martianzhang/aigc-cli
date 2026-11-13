package video

import (
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// printVideoResult prints the H3 result footer for generated or converted
// videos. taskID is empty for synchronous providers and local conversions.
func printVideoResult(req *types.VideoGenerateRequest, p *provider.EffectiveProvider, saved []string, taskID string) {
	providerName := ""
	if p != nil {
		providerName = p.ProviderType.String()
	}
	model := ""
	if req != nil {
		model = req.Model
	}
	next := ""
	if taskID != "" && p != nil {
		switch p.ProviderType {
		case provider.APIMart:
			next = "aigc-cli task " + taskID
		case provider.OpenRouter:
			next = "aigc-cli video --job-id " + taskID
		}
	}
	service.PrintResultSummary(service.ResultSummary{
		Files:    saved,
		Model:    model,
		Provider: providerName,
		TaskID:   taskID,
		Next:     next,
	})
}
