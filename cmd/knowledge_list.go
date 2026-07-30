package cmd

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/vault"
	"github.com/spf13/cobra"
)

type tblRow struct {
	cells []string
}

// kbListCmd lists all documents in the knowledge base.
var (
	kbListLimit  int
	kbListOffset int
)

var kbListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List all documents",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultEnabled {
			return listVault()
		}
		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		limit := kbListLimit
		if limit <= 0 {
			limit = 9999 // unlimited
		}

		project := ""
		if !isGlobalScope(cmd) {
			project = detectProject(cmd)
		}

		total, err := store.CountDocuments()
		if err != nil {
			return fmt.Errorf("count: %w", err)
		}

		docs, err := store.ListDocuments(limit, kbListOffset, project)
		if err != nil {
			return fmt.Errorf("list: %w", err)
		}

		if len(docs) == 0 {
			fmt.Fprintln(os.Stderr, "Knowledge base is empty. Add documents with 'kb add' or 'kb fetch'.")
			return nil
		}

		rows := []tblRow{{cells: []string{"ID", "Title", "Source", "Added", "Size"}}}
		for _, d := range docs {
			added := d.CreatedAt.Format("2006-01-02 15:04")
			if d.CreatedAt.IsZero() {
				added = "-"
			}
			rows = append(rows, tblRow{cells: []string{
				d.ID[:12],
				d.Title,
				displaySource(d.URL, d.FilePath),
				added,
				fmt.Sprintf("%.1f KB", float64(d.Size)/1024),
			}})
		}
		renderTable(os.Stdout, rows)
		shown := len(docs)
		if shown > 0 && kbListOffset+shown < total {
			fmt.Fprintf(os.Stderr, "\nShowing %d-%d of %d documents. Use --limit and --offset to navigate.\n",
				kbListOffset+1, kbListOffset+shown, total)
		} else if total > 0 {
			fmt.Fprintf(os.Stderr, "\nTotal: %d documents.\n", total)
		}
		return nil
	},
}

// renderTable renders a table with aligned columns, correctly handling CJK width.
func renderTable(w *os.File, rows []tblRow) {
	if len(rows) == 0 {
		return
	}
	colCount := len(rows[0].cells)

	// Compute max visual width per column across all rows
	maxWidths := make([]int, colCount)
	for _, r := range rows {
		for i, c := range r.cells {
			w := runewidth(c)
			if w > maxWidths[i] {
				maxWidths[i] = w
			}
		}
	}

	// Separator row
	sep := make([]string, colCount)
	for i, mw := range maxWidths {
		sep[i] = strings.Repeat("─", mw)
	}

	// Render header
	renderRow(w, rows[0].cells, maxWidths)
	// Render separator
	fmt.Fprintf(w, " %s\n", strings.Join(sep, "  "))
	// Render data rows
	for _, r := range rows[1:] {
		renderRow(w, r.cells, maxWidths)
	}
}

func renderRow(w *os.File, cells []string, widths []int) {
	parts := make([]string, len(cells))
	for i, c := range cells {
		parts[i] = padWidth(c, widths[i])
	}
	fmt.Fprintf(w, " %s\n", strings.Join(parts, "  "))
}

// runewidth returns the visual terminal width (CJK=2, others=1).
func runewidth(s string) int {
	w := 0
	for _, r := range s {
		w += text.RuneWidth(r)
	}
	return w
}

// padWidth pads s with spaces on the right to match the given visual width.
func padWidth(s string, w int) string {
	if n := w - runewidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// displaySource returns a short human-readable source label from a URL or file path.
func displaySource(rawURL, filePath string) string {
	if rawURL != "" {
		if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
			return u.Host
		}
		return rawURL
	}
	if filePath != "" {
		return "file"
	}
	return "-"
}

func listVault() error {
	store, err := openKBStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	docs, err := store.ListDocuments(100, 0, "")
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}

	v, err := vault.Open(vaultBaseDir)
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}
	vaultDocs, err := v.List()
	if err != nil {
		return fmt.Errorf("list vault: %w", err)
	}
	vaultIDs := make(map[string]bool)
	for _, d := range vaultDocs {
		vaultIDs[d.ID] = true
	}

	var vaultItems []knowledge.Document
	for _, d := range docs {
		if vaultIDs[d.ID] {
			vaultItems = append(vaultItems, d)
		}
	}

	if len(vaultItems) == 0 {
		fmt.Fprintln(os.Stderr, "Vault is empty. Use 'kb add --vault' to add files.")
		return nil
	}

	rows := []tblRow{{cells: []string{"ID", "Title", "Source"}}}
	for _, d := range vaultItems {
		source := d.URL
		if source == "" {
			source = d.FilePath
		}
		if source == "" {
			source = "(no source)"
		}
		rows = append(rows, tblRow{cells: []string{d.ID[:12], d.Title, source}})
	}
	renderTable(os.Stdout, rows)
	return nil
}

func init() {
	kbListCmd.Flags().IntVar(&kbListLimit, "limit", 100, "Max documents to show (0 for unlimited)")
	kbListCmd.Flags().IntVar(&kbListOffset, "offset", 0, "Number of documents to skip")
}
