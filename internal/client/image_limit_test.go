package client

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/httpx"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// TestImageBodyLimitErrorMentionsBilling pins the operator-facing advice: an
// oversized image response may already be a billed generation, so the user
// must raise the cap instead of retrying blindly.
func TestImageBodyLimitErrorMentionsBilling(t *testing.T) {
	t.Setenv(httpx.ImageBodyEnv, "1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.Copy(w, strings.NewReader(strings.Repeat("a", 2<<20)))
	}))
	defer srv.Close()

	c := New("k", srv.URL, "")
	_, err := c.OpenRouterDedicatedImage(&types.GenerateRequest{Model: "m", Prompt: "p"})
	if err == nil {
		t.Fatal("OpenRouterDedicatedImage() error = nil, want the body cap to fire")
	}
	if !errors.Is(err, httpx.ErrBodyTooLarge) {
		t.Errorf("error = %v, want ErrBodyTooLarge", err)
	}
	for _, want := range []string{"may already have been generated", httpx.ImageBodyEnv} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want %q", err, want)
		}
	}
}
