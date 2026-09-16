package task

import (
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// QueryTextDefault queries a task using the process-global shared config.
func QueryTextDefault(taskID string) (string, error) {
	var providers map[string]*types.NamedProvider
	if options.Shared.Cfg != nil {
		providers = options.Shared.Cfg.Providers
	}
	return QueryText(Deps{
		ResolveProvider: options.Shared.ResolveProvider,
		Providers:       providers,
		DownloadImages: func(images []types.ImageResult, id string) ([]string, error) {
			return service.DownloadImages(images, options.Shared.OutputDir, id)
		},
		DownloadVideos: func(videos []types.VideoResult, id string) ([]string, error) {
			return service.DownloadVideos(videos, options.Shared.OutputDir, id)
		},
	}, taskID)
}
