package knowledge

import (
	"fmt"
	"net/url"
	"os"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/terminal"
	"github.com/martianzhang/aigc-cli/internal/ui"
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
	Use:   "list",
	Short: "List all documents",
	Example: `  aigc-cli kb list
  aigc-cli kb list --limit 20
  aigc-cli kb list --all`,
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
		defer func() { _ = store.Close() }()

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
			ui.Eprintln("Knowledge base is empty. Add documents with 'kb add' or 'kb fetch'.")
			return nil
		}

		rows := []tblRow{{cells: []string{"ID", "Title", "Source", "Added", "Size"}}}
		for _, d := range docs {
			added := d.CreatedAt.Format("2006-01-02 15:04")
			if d.CreatedAt.IsZero() {
				added = "-"
			}
			rows = append(rows, tblRow{cells: []string{
				knowledge.ShortID(d.ID),
				d.Title,
				displaySource(d.URL, d.FilePath),
				added,
				fmt.Sprintf("%.1f KB", float64(d.Size)/1024),
			}})
		}
		renderTable(os.Stdout, rows)
		shown := len(docs)
		if shown > 0 && kbListOffset+shown < total {
			ui.Eprintf("\nShowing %d-%d of %d documents. Use --limit and --offset to navigate.\n",
				kbListOffset+1, kbListOffset+shown, total)
		} else if total > 0 {
			ui.Eprintf("\nTotal: %d documents.\n", total)
		}
		return nil
	},
}

// renderTable renders a table with aligned columns, correctly handling CJK width.
func renderTable(w *os.File, rows []tblRow) {
	if len(rows) == 0 {
		return
	}
	data := make([][]string, 0, len(rows)-1)
	for _, r := range rows[1:] {
		data = append(data, r.cells)
	}
	_ = terminal.WriteTable(w, rows[0].cells, data, terminal.TableOptions{
		Indent:    " ",
		Separator: true,
	})
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
	defer func() { _ = store.Close() }()

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
		ui.Eprintln("Vault is empty. Use 'kb add --vault' to add files.")
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
		rows = append(rows, tblRow{cells: []string{knowledge.ShortID(d.ID), d.Title, source}})
	}
	renderTable(os.Stdout, rows)
	return nil
}

func init() {
	kbListCmd.Flags().IntVar(&kbListLimit, "limit", 100, "Max documents to show (0 for unlimited)")
	kbListCmd.Flags().IntVar(&kbListOffset, "offset", 0, "Number of documents to skip")
}
