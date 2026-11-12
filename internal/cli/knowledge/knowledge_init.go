package knowledge

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/ui"
	"github.com/spf13/cobra"
)

var kbInitForceModel bool

var kbInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the knowledge base",
	Long: `Creates the SQLite database, directory structure, and
downloads the embedding model.

Run this once before using other kb commands. It's idempotent —
re-running won't overwrite existing data or models.

Use --force to re-download models.`,
	Example: `  aigc-cli kb init
  aigc-cli kb init --force`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer func() { _ = store.Close() }()

		count, _ := store.CountDocuments()
		ui.Eprintf("Knowledge base initialized at %s\n", kbBaseDir)
		if count > 0 {
			ui.Eprintf("Existing documents: %d\n", count)
		}

		docsDir := kbBaseDir + "/docs"
		if err := os.MkdirAll(docsDir, 0755); err != nil {
			return fmt.Errorf("create docs dir: %w", err)
		}

		// Download embedding model to shared models dir
		modelsDir := filepath.Join(options.ConfigDir(), "models")
		ui.Eprintf("Downloading embedding model...\n")
		if err := knowledge.InitEmbedModel(modelsDir, kbInitForceModel); err != nil {
			ui.Eprintf("Warning: model download failed: %v\n", err)
			ui.Eprintf("  The knowledge base will work without it, but search quality\n")
			ui.Eprintf("  will be limited until 'kb init' completes successfully.\n")
			ui.Eprintf("  Retry with: aigc-cli kb init --force\n")
		} else {
			ui.Eprintf("Embedding model ready.\n")
		}

		return nil
	},
}

func init() {
	kbInitCmd.Flags().BoolVar(&kbInitForceModel, "force", false, "Re-download models even if they exist")
}
