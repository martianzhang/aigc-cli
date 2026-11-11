package terminal

import (
	"io"
	"strings"

	prettytext "github.com/jedib0t/go-pretty/v6/text"
)

func init() {
	// Treat East Asian characters as two cells everywhere this package renders.
	// The setting is process-wide in go-pretty; do it once instead of on every
	// table, which would race under a concurrent server.
	prettytext.OverrideRuneWidthEastAsianWidth(true)
}

// TableOptions controls the layout produced by WriteTable.
type TableOptions struct {
	// Indent is written before every line (header, separator and rows).
	Indent string
	// Padding separates adjacent columns. Empty means two spaces.
	Padding string
	// PadLast pads the final column too. The default leaves it unpadded so
	// long trailing values (regexes, error messages) do not sprout spaces.
	PadLast bool
	// Separator draws a "─" rule under the header. The rule uses the same
	// display width as each column.
	Separator bool
}

// WriteTable writes headers and rows as left-aligned columns measured by
// terminal display width. CJK characters and emoji occupy two cells, so plain
// text/tabwriter (which counts bytes or runes) misaligns them.
//
// A nil/empty headers slice omits the header row. Rows shorter than the widest
// row are padded with empty cells; an empty options value is valid.
func WriteTable(w io.Writer, headers []string, rows [][]string, opts TableOptions) error {
	padding := opts.Padding
	if padding == "" {
		padding = "  "
	}
	if len(headers) == 0 && len(rows) == 0 {
		return nil
	}

	cols := len(headers)
	for _, row := range rows {
		if len(row) > cols {
			cols = len(row)
		}
	}

	widths := make([]int, cols)
	measure := func(row []string) {
		for i := 0; i < cols && i < len(row); i++ {
			if width := displayWidth(row[i]); width > widths[i] {
				widths[i] = width
			}
		}
	}
	measure(headers)
	for _, row := range rows {
		measure(row)
	}

	writeRow := func(row []string) error {
		if _, err := io.WriteString(w, opts.Indent); err != nil {
			return err
		}
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			last := i == cols-1
			if last && !opts.PadLast {
				_, err := io.WriteString(w, cell+"\n")
				return err
			}
			if _, err := io.WriteString(w, PadRight(cell, widths[i])); err != nil {
				return err
			}
			if !last {
				if _, err := io.WriteString(w, padding); err != nil {
					return err
				}
			}
		}
		_, err := io.WriteString(w, "\n")
		return err
	}

	if len(headers) > 0 {
		if err := writeRow(headers); err != nil {
			return err
		}
		if opts.Separator {
			parts := make([]string, cols)
			for i, width := range widths {
				parts[i] = strings.Repeat("─", width)
			}
			if _, err := io.WriteString(w, opts.Indent+strings.Join(parts, padding)+"\n"); err != nil {
				return err
			}
		}
	}
	for _, row := range rows {
		if err := writeRow(row); err != nil {
			return err
		}
	}
	return nil
}

// DisplayWidth returns the number of terminal cells s occupies. ANSI escape
// sequences are ignored and East Asian wide characters count as two cells.
func DisplayWidth(s string) int {
	return displayWidth(s)
}

func displayWidth(s string) int {
	return prettytext.StringWidthWithoutEscSequences(s)
}

// PadRight appends spaces until s occupies at least width terminal cells.
// Strings that are already wide enough are returned unchanged.
func PadRight(s string, width int) string {
	if n := width - displayWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
