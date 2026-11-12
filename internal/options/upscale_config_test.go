package options

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestUpscaleProviderResolution(t *testing.T) {
	saved := Shared
	defer func() { Shared = saved }()

	Shared = &SharedConfig{
		Cfg: &types.Config{
			Defaults: &types.ConfigDefaults{
				Upscale: &types.UpscaleDefaults{Provider: "my-local", Model: "real-cugan-2x"},
			},
			Providers: map[string]*types.NamedProvider{
				"my-local": {Type: types.ProviderLocal, ModelsDir: "/data/models"},
			},
		},
	}

	if got := UpscaleModel(); got != "real-cugan-2x" {
		t.Errorf("UpscaleModel() = %q, want real-cugan-2x", got)
	}
	if got := UpscaleModelsDir(); got != "/data/models" {
		t.Errorf("UpscaleModelsDir() = %q, want /data/models", got)
	}
}

func TestUpscaleResolutionUnset(t *testing.T) {
	saved := Shared
	defer func() { Shared = saved }()

	Shared = &SharedConfig{Cfg: &types.Config{}}

	if got := UpscaleModel(); got != "" {
		t.Errorf("UpscaleModel() = %q, want empty when unconfigured", got)
	}
	if got := UpscaleModelsDir(); got != "" {
		t.Errorf("UpscaleModelsDir() = %q, want empty when unconfigured", got)
	}
}
