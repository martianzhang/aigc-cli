package knowledge

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// kbRmCmd removes a document from the knowledge base.
var kbRmCmd = &cobra.Command{
	Use:   "rm <doc-id> [doc-id ...]",
	Short: "Remove document(s) from the knowledge base",
	Long: `Remove one or more documents by their ID prefix (as shown in 'kb list').

Use --all to remove all documents.`,
	Example: `  aigc-cli kb rm a1b2c3d4e5f6
  aigc-cli kb rm a1b2c3d4e5f6 b2c3d4e5f6a1`,
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer func() { _ = store.Close() }()

		for _, arg := range args {
			if arg == "--all" || arg == "all" {
				// Delete all
				docs, err := store.ListDocuments(10000, 0, "")
				if err != nil {
					return fmt.Errorf("list: %w", err)
				}
				for _, d := range docs {
					if err := store.DeleteDocument(d.ID); err != nil {
						ui.Eprintf("Error deleting %s: %v\n", knowledge.ShortID(d.ID), err)
					} else {
						ui.Eprintf("  Removed: %s\n", d.Title)
					}
				}
				return nil
			}

			doc, err := findDocByPrefix(store, arg)
			if err != nil {
				return err
			}
			if doc == nil {
				ui.Eprintf("  Not found: %s\n", arg)
				continue
			}
			if err := store.DeleteDocument(doc.ID); err != nil {
				return fmt.Errorf("delete: %w", err)
			}
			ui.Eprintf("  Removed: %s\n", doc.Title)
		}
		return nil
	},
}
