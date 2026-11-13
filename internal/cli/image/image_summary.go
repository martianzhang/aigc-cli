package image

import (
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// printImageSummary prints the H3 result footer shared by every image runner.
// taskID is empty for synchronous providers because their generated filename
// prefix is not a resumable task id.
func printImageSummary(ctx *imageDispatchCtx, req *types.GenerateRequest, saved []string, taskID string, cost float64) {
	summary := service.ResultSummary{
		Files:  saved,
		Model:  req.Model,
		TaskID: taskID,
	}
	if ctx != nil {
		if p := ctx.providerFor(); p != nil {
			summary.Provider = p.ProviderType.String()
			if p.Type == types.ProviderOllama || provider.IsLocalEndpoint(p.BaseURL) {
				summary.Model = "local: " + req.Model
				summary.Provider = ""
			}
		}
	}
	if cost > 0 {
		summary.Cost = cost
		summary.HasCost = true
	}
	if ctx != nil && ctx.isAPIMart && taskID != "" {
		summary.Next = "aigc-cli task " + taskID
	}
	service.PrintResultSummary(summary)
}

// usageCost extracts a non-negative cost from an optional usage block.
func usageCost(usage *types.OpenAIImageUsage) float64 {
	if usage == nil || usage.Cost <= 0 {
		return 0
	}
	return usage.Cost
}
