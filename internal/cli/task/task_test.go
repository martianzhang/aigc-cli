package task

import (
	"strings"
	"testing"
)

func TestQueryText_nonAPIMart(t *testing.T) {
	t.Run("unknown provider", func(t *testing.T) {
		_, err := QueryText(Deps{}, "task_1")
		if err == nil || !strings.Contains(err.Error(), "only supported on APIMart") {
			t.Fatalf("got err=%v, want APIMart-only error", err)
		}
	})

	t.Run("openrouter hint", func(t *testing.T) {
		_, err := QueryText(Deps{APIBase: "https://openrouter.ai/api/v1"}, "task_1")
		if err == nil || !strings.Contains(err.Error(), "--job-id") {
			t.Fatalf("got err=%v, want OpenRouter hint", err)
		}
	})
}
