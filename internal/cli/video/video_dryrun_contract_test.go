package video

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/testutil"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestDryRunPreviewMakesNoRequests(t *testing.T) {
	defer testutil.FailOnNetwork(t)()

	_ = buildVideoCurl(&types.VideoGenerateRequest{Prompt: "p", Model: "m"})
}
