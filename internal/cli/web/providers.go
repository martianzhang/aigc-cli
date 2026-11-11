package web

import (
	"io"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/terminal"
)

// writeProviderList prints the configured provider registry: name, type,
// search/fetch/media capability and availability (including construction errors).
func writeProviderList(w io.Writer, router *search.Router) error {
	headers := []string{"NAME", "TYPE", "SEARCH", "FETCH", "MEDIA", "STATUS"}
	rows := make([][]string, 0)
	for _, status := range router.Statuses() {
		typ := status.Type
		if typ == "" {
			typ = "-"
		}
		if !status.Available {
			rows = append(rows, []string{status.Name, typ, "-", "-", "-", "unavailable: " + status.Err})
			continue
		}
		fetch := "no"
		if status.Fetch {
			fetch = "yes"
		}
		media := "-"
		if status.Media {
			media = strings.Join(status.MediaTypes, ",")
		}
		rows = append(rows, []string{status.Name, typ, "yes", fetch, media, "available"})
	}
	return terminal.WriteTable(w, headers, rows, terminal.TableOptions{})
}
