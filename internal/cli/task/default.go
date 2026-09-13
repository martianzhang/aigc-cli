package task

import (
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// QueryTextDefault queries a task using the process-global shared config.
func QueryTextDefault(taskID string) (string, error) {
	return QueryText(Deps{
		APIBase:   options.Shared.APIBase,
		APIKey:    options.Shared.APIKey,
		HTTPProxy: options.Shared.HTTPProxy,
		DownloadImages: func(images []types.ImageResult, id string) ([]string, error) {
			return service.DownloadImages(images, options.Shared.OutputDir, id)
		},
		DownloadVideos: func(videos []types.VideoResult, id string) ([]string, error) {
			return service.DownloadVideos(videos, options.Shared.OutputDir, id)
		},
	}, taskID)
}
