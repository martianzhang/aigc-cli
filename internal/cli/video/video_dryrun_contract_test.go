package video

import (
	"errors"
	"net/http"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.t.Error("dry-run preview performed a network request")
	return nil, errors.New("no network in dry-run")
}

func TestDryRunPreviewMakesNoRequests(t *testing.T) {
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: failingTransport{t: t}}
	t.Cleanup(func() { http.DefaultClient = old })

	_ = buildVideoCurl(&types.VideoGenerateRequest{Prompt: "p", Model: "m"})
}
