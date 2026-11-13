package upscale

import "github.com/martianzhang/aigc-cli/internal/service"

// printUpscaleResult prints the H3 footer for a local upscale result.
func printUpscaleResult(outPath, modelID string) {
	service.PrintResultSummary(service.ResultSummary{
		Files: []string{outPath},
		Model: "local: " + modelID,
	})
}
