package cmd

import (
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/vault"
	"github.com/spf13/cobra"
)

// kbListCmd lists all documents in the knowledge base.
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

		project := ""
		if !isGlobalScope(cmd) {
			project = detectProject(cmd)
		}

		docs, err := store.ListDocuments(100, 0, project)
		if err != nil {
			return fmt.Errorf("list: %w", err)
		}

		if len(docs) == 0 {
			fmt.Fprintln(os.Stderr, "Knowledge base is empty. Add documents with 'kb add' or 'kb fetch'.")
			return nil
		}

		fmt.Printf("%-12s %-30s %-16s %-8s %s\n", "ID", "Title", "Scope", "Size", "Source")
		fmt.Println("------------------------------------------------------------------------")
		for _, d := range docs {
			source := d.URL
			if source == "" {
				source = d.FilePath
			}
			if source == "" {
				source = "(no source)"
			}
			id := d.ID[:12]
			title := d.Title
			if len(title) > 28 {
				title = title[:28] + "..."
			}
			scope := formatProject(d.Project)
			src := source
			if len(src) > 18 {
				src = src[:18] + "..."
			}
			size := fmt.Sprintf("%.1f KB", float64(d.Size)/1024)
			fmt.Printf("%-12s %-30s %-16s %-8s %s\n", id, title, scope, size, src)
		}
		return nil
	},
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

	// Filter vault docs (where id is in vault directory)
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

	fmt.Printf("%-12s %-30s %s\n", "ID", "Title", "Source")
	fmt.Println("----------------------------------------------------------")
	for _, d := range vaultItems {
		source := d.URL
		if source == "" {
			source = d.FilePath
		}
		if source == "" {
			source = "(no source)"
		}
		fmt.Printf("%-12s %-30s %s\n", d.ID[:12], d.Title, source)
	}
	return nil
}
