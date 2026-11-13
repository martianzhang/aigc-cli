package audio

import (
	"testing"

	"github.com/martianzhang/aigc-cli/internal/testutil"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestDryRunPreviewMakesNoRequests(t *testing.T) {
	defer testutil.FailOnNetwork(t)()

	_ = buildAudioSpeechCurl(&types.AudioSpeechRequest{Model: "m", Voice: "v", ResponseFormat: "mp3"})
}
