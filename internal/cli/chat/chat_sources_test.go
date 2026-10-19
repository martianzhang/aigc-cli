package chat

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestPrintSourcesRendersCitationsToStderr(t *testing.T) {
	var buf bytes.Buffer
	restore := options.SetStderr(&buf)
	t.Cleanup(restore)

	res := &types.ChatResponse{Choices: []types.ChatChoice{{Message: types.ChatMessage{
		Role: "assistant",
		Annotations: []types.ChatAnnotation{
			{Type: "url_citation", URL: "https://a.example", Title: "Title A", SiteName: "Site A"},
			{Type: "url_citation", URL: "https://b.example"}, // URL only
			{Type: "url_citation", Title: "no url"},          // dropped
		},
	}}}}

	printSources(res)

	out := buf.String()
	if !strings.Contains(out, "Sources (2)") {
		t.Errorf("output = %q, want a Sources header counting only usable citations", out)
	}
	if !strings.Contains(out, "Title A (Site A) — https://a.example") {
		t.Errorf("output = %q, want title+site+url line", out)
	}
	if !strings.Contains(out, "https://b.example") {
		t.Errorf("output = %q, want the URL-only citation", out)
	}
	if strings.Contains(out, "no url") {
		t.Errorf("output = %q, want URL-less annotations skipped", out)
	}
}

func TestPrintSourcesCapsWithoutVerbose(t *testing.T) {
	prevVerbose := options.Shared.Verbose
	options.Shared.Verbose = false
	t.Cleanup(func() { options.Shared.Verbose = prevVerbose })

	var buf bytes.Buffer
	restore := options.SetStderr(&buf)
	t.Cleanup(restore)

	var anns []types.ChatAnnotation
	for i := 0; i < sourcesDefaultCap+3; i++ {
		anns = append(anns, types.ChatAnnotation{
			Type: "url_citation",
			URL:  fmt.Sprintf("https://example.com/%d", i),
		})
	}
	printSources(&types.ChatResponse{Choices: []types.ChatChoice{{Message: types.ChatMessage{Annotations: anns}}}})

	out := buf.String()
	if !strings.Contains(out, fmt.Sprintf("Sources (%d)", sourcesDefaultCap+3)) {
		t.Errorf("output = %q, want the true total in the header", out)
	}
	if !strings.Contains(out, "… and 3 more (use -v to list all)") {
		t.Errorf("output = %q, want the truncation hint", out)
	}
	if strings.Contains(out, "https://example.com/10") {
		t.Errorf("output = %q, want entries beyond the cap hidden", out)
	}
}

func TestPrintSourcesSilentWithoutAnnotations(t *testing.T) {
	var buf bytes.Buffer
	restore := options.SetStderr(&buf)
	t.Cleanup(restore)

	printSources(nil)
	printSources(&types.ChatResponse{Choices: []types.ChatChoice{{Message: types.ChatMessage{Content: "hi"}}}})

	if buf.Len() != 0 {
		t.Errorf("output = %q, want nothing without annotations", buf.String())
	}
}

func TestWebSearchLineIsMiMoOnly(t *testing.T) {
	usage := &types.ChatUsage{WebSearchUsage: &types.WebSearchUsage{ToolUsage: 2, PageUsage: 7}}
	mimo := &provider.EffectiveProvider{BaseURL: "https://api.xiaomimimo.com/v1", ProviderType: provider.MiMo}
	openai := &provider.EffectiveProvider{BaseURL: "https://api.openai.com/v1", ProviderType: provider.OpenAI}

	line := webSearchLine(mimo, usage)
	if !strings.Contains(line, "2 call(s)") || !strings.Contains(line, "7 page(s)") {
		t.Errorf("webSearchLine(mimo) = %q, want both counters", line)
	}
	if got := webSearchLine(openai, usage); got != "" {
		t.Errorf("webSearchLine(openai) = %q, want empty (MiMo-only counter)", got)
	}
	if got := webSearchLine(mimo, &types.ChatUsage{}); got != "" {
		t.Errorf("webSearchLine(mimo, no usage) = %q, want empty", got)
	}
	if got := webSearchLine(nil, usage); got != "" {
		t.Errorf("webSearchLine(nil) = %q, want empty", got)
	}
}
