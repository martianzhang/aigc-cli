package web

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/martianzhang/aigc-cli/internal/search"
)

// writeProviderList prints the configured provider registry: name, type,
// search/fetch capability and availability (including construction errors).
func writeProviderList(w io.Writer, router *search.Router) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "NAME\tTYPE\tSEARCH\tFETCH\tSTATUS"); err != nil {
		return err
	}
	for _, status := range router.Statuses() {
		typ := status.Type
		if typ == "" {
			typ = "-"
		}
		if !status.Available {
			if _, err := fmt.Fprintf(tw, "%s\t%s\t-\t-\tunavailable: %s\n", status.Name, typ, status.Err); err != nil {
				return err
			}
			continue
		}
		fetch := "no"
		if status.Fetch {
			fetch = "yes"
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\tyes\t%s\tavailable\n", status.Name, typ, fetch); err != nil {
			return err
		}
	}
	return tw.Flush()
}
