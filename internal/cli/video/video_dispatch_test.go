package video

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func firstVideoMatchRunName(req *types.VideoGenerateRequest, ctx *videoDispatchCtx) string {
	for _, s := range videoStrategies {
		if s.match(req, ctx) {
			return runtime.FuncForPC(reflect.ValueOf(s.run).Pointer()).Name()
		}
	}
	return ""
}

func TestVideoStrategyDispatch_AllProviders(t *testing.T) {
	req := &types.VideoGenerateRequest{Model: "m", Prompt: "p"}

	tests := []struct {
		name     string
		ctx      *videoDispatchCtx
		wantFunc string
	}{
		{"OpenRouter uses dedicated video API", &videoDispatchCtx{isOpenRouter: true}, "runOpenRouterVideo"},
		{"Agnes uses async task API", &videoDispatchCtx{isAgnes: true}, "runAgnesVideo"},
		{"Yunwu uses unified video API", &videoDispatchCtx{isYunwu: true}, "runYunwuVideo"},
		{"APIMart default uses async task API", &videoDispatchCtx{}, "runAPIMartVideo"},
		{"generic default falls back to APIMart", &videoDispatchCtx{}, "runAPIMartVideo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstVideoMatchRunName(req, tc.ctx)
			if !strings.HasSuffix(got, tc.wantFunc) {
				t.Errorf("first match = %q, want suffix %q", got, tc.wantFunc)
			}
		})
	}
}
