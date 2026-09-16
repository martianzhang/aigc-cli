package cmd

import (
	"github.com/martianzhang/aigc-cli/internal/cli/task"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// taskDeps resolves the task command's dependencies from the loaded config.
func taskDeps() task.Deps {
	var providers map[string]*types.NamedProvider
	if shared.Cfg != nil {
		providers = shared.Cfg.Providers
	}
	return task.Deps{
		ResolveProvider: shared.ResolveProvider,
		Providers:       providers,
		DownloadImages: func(images []types.ImageResult, id string) ([]string, error) {
			return service.DownloadImages(images, shared.OutputDir, id)
		},
		DownloadVideos: func(videos []types.VideoResult, id string) ([]string, error) {
			return service.DownloadVideos(videos, shared.OutputDir, id)
		},
	}
}

func init() {
	rootCmd.AddCommand(task.NewCommand(taskDeps))
}
