package web

import (
	"fmt"
	"io"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/search"
)

// writeDryRun prints the equivalent curl command for the selected provider
// without sending any request.
func writeDryRun(w io.Writer, query, provider string, limit int, opts search.Options, router *search.Router) error {
	p, ok := router.GetProvider(provider)
	if !ok {
		return fmt.Errorf("search provider %q is not configured", provider)
	}
	previewer, ok := p.(search.Previewer)
	if !ok {
		return fmt.Errorf("provider %q does not provide a curl preview", provider)
	}
	plan, err := previewer.Preview(query, limit, opts)
	if err != nil {
		return fmt.Errorf("preview %s: %w", provider, err)
	}

	fmt.Fprintln(w, "# dry run: no request sent")
	fmt.Fprintf(w, "# query: %s\n", oneLine(query))
	fmt.Fprintf(w, "# provider: %s\n", provider)
	_, err = fmt.Fprintln(w, plan.RenderCurls(""))
	return err
}

// oneLine keeps a value safe for a leading "# " comment.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
