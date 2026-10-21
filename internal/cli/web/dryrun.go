package web

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/search"
)

// writeDryRun prints the equivalent curl command for each candidate provider
// without sending any request. Under `auto` several vendors may be candidates;
// the final pick is weighted, so each candidate is shown.
func writeDryRun(w io.Writer, query, strategy string, preferred []string, limit int, inject json.RawMessage, router *search.Router) error {
	candidates := router.Candidates(strategy, preferred)
	if len(candidates) == 0 {
		return fmt.Errorf("no providers available (all exhausted quota or none configured)")
	}

	fmt.Fprintln(w, "# dry run: no request sent")
	fmt.Fprintf(w, "# query: %s\n", oneLine(query))
	fmt.Fprintf(w, "# strategy: %s\n", strategy)
	for i, name := range candidates {
		provider, ok := router.GetProvider(name)
		if !ok {
			continue
		}
		previewer, ok := provider.(search.Previewer)
		if !ok {
			fmt.Fprintf(w, "# provider: %s (candidate %d/%d): no curl preview available\n", name, i+1, len(candidates))
			continue
		}
		label := fmt.Sprintf("# provider: %s (candidate %d/%d", name, i+1, len(candidates))
		if len(candidates) > 1 {
			label += "; final pick is weighted"
		}
		label += ")"
		plan, err := previewer.Preview(query, limit, search.Options{Extra: inject})
		if err != nil {
			fmt.Fprintf(w, "# provider: %s (candidate %d/%d): %v\n", name, i+1, len(candidates), err)
			continue
		}
		fmt.Fprintln(w, label)
		fmt.Fprintln(w, plan.RenderCurls(""))
	}
	return nil
}

// oneLine keeps a value safe for a leading "# " comment.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
