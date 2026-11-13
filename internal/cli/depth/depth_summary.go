package depth

import "github.com/martianzhang/aigc-cli/internal/service"

// printDepthResult prints the H3 footer for a local depth result.
func printDepthResult(out, modelID string) {
	service.PrintResultSummary(service.ResultSummary{
		Files: []string{out},
		Model: "local: " + modelID,
	})
}
