package knowledge

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/ui"
	"github.com/spf13/cobra"
)

var kbResetForce bool

var kbResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete all data and reinitialize",
	Long: `Remove all documents, chunks, and embeddings from the knowledge base,
then reinitialize the database. The docs/ directory is also cleaned.

Use --force to skip confirmation.`,
	Example: `  aigc-cli kb reset
  aigc-cli kb reset --force`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !kbResetForce {
			ui.Eprintf("This will delete ALL knowledge base data. Continue? [y/N]: ")
			var response string
			_, _ = fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				ui.Eprintln("Cancelled.")
				return nil
			}
		}

		// Close any open store (none at this point in the command)
		// Remove the old database
		dbPath := filepath.Join(kbBaseDir, "knowledge.db")
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove db: %w", err)
		}
		ui.Eprintf("  Removed: %s\n", dbPath)

		// Remove WAL and SHM files. They may not exist; cleanup is best-effort.
		for _, ext := range []string{"-wal", "-shm"} {
			_ = os.Remove(dbPath + ext)
		}

		// Remove docs/ directory
		docsDir := filepath.Join(kbBaseDir, "docs")
		if err := os.RemoveAll(docsDir); err != nil {
			return fmt.Errorf("remove docs: %w", err)
		}
		ui.Eprintf("  Removed: %s\n", docsDir)

		// Reinitialize
		ui.Eprintf("Reinitializing...\n")
		if err := ensureKBDir(); err != nil {
			return err
		}

		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("reinitialize: %w", err)
		}
		_ = store.Close()

		ui.Eprintf("Knowledge base reset complete.\n")
		return nil
	},
}

func init() {
	kbResetCmd.Flags().BoolVar(&kbResetForce, "force", false, "Skip confirmation")
}
