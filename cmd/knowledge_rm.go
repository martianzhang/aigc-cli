package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// kbRmCmd removes a document from the knowledge base.
var kbRmCmd = &cobra.Command{
	Use:   "rm <doc-id> [doc-id ...]",
	Short: "Remove document(s) from the knowledge base",
	Long: `Remove one or more documents by their ID prefix (as shown in 'kb list').

Use --all to remove all documents.`,
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		for _, arg := range args {
			if arg == "--all" || arg == "all" {
				// Delete all
				docs, err := store.ListDocuments(10000, 0, "")
				if err != nil {
					return fmt.Errorf("list: %w", err)
				}
				for _, d := range docs {
					if err := store.DeleteDocument(d.ID); err != nil {
						fmt.Fprintf(os.Stderr, "Error deleting %s: %v\n", d.ID[:12], err)
					} else {
						fmt.Fprintf(os.Stderr, "  Removed: %s\n", d.Title)
					}
				}
				return nil
			}

			doc, err := findDocByPrefix(store, arg)
			if err != nil {
				return err
			}
			if doc == nil {
				fmt.Fprintf(os.Stderr, "  Not found: %s\n", arg)
				continue
			}
			if err := store.DeleteDocument(doc.ID); err != nil {
				return fmt.Errorf("delete: %w", err)
			}
			fmt.Fprintf(os.Stderr, "  Removed: %s\n", doc.Title)
		}
		return nil
	},
}
