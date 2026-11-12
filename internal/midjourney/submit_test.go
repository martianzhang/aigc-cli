package midjourney

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestResultSummary(t *testing.T) {
	if got := ResultSummary(nil); got != "No result returned." {
		t.Fatalf("nil summary = %q", got)
	}
	got := ResultSummary(&types.MJTaskData{
		Action:      "imagine",
		Status:      "SUCCESS",
		ImageURLs:   []string{"https://example.com/1.png"},
		Prompt:      "a cat",
		ActualTime:  12,
		Cost:        0.01,
		CreditsCost: 0.02,
	})
	for _, want := range []string{"Action: imagine", "Image 1:", "Prompt: a cat", "Completed in 12s"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q:\n%s", want, got)
		}
	}
}
