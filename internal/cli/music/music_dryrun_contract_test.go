package music

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/testutil"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestDryRunPreviewMakesNoRequests(t *testing.T) {
	defer testutil.FailOnNetwork(t)()

	_ = buildMusicCurl("https://example.com", "key", &types.MusicGenerateRequest{Prompt: "p"})
}
