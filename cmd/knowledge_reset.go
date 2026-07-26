package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var kbResetForce bool

var kbResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete all data and reinitialize",
	Long: `Remove all documents, chunks, and embeddings from the knowledge base,
then reinitialize the database. The docs/ directory is also cleaned.

Use --force to skip confirmation.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !kbResetForce {
			fmt.Fprintf(os.Stderr, "This will delete ALL knowledge base data. Continue? [y/N]: ")
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" {
				fmt.Fprintln(os.Stderr, "Cancelled.")
				return nil
			}
		}

		// Close any open store (none at this point in the command)
		// Remove the old database
		dbPath := filepath.Join(kbBaseDir, "knowledge.db")
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove db: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  Removed: %s\n", dbPath)

		// Remove WAL and SHM files
		for _, ext := range []string{"-wal", "-shm"} {
			os.Remove(dbPath + ext)
		}

		// Remove docs/ directory
		docsDir := filepath.Join(kbBaseDir, "docs")
		if err := os.RemoveAll(docsDir); err != nil {
			return fmt.Errorf("remove docs: %w", err)
		}
		fmt.Fprintf(os.Stderr, "  Removed: %s\n", docsDir)

		// Reinitialize
		fmt.Fprintf(os.Stderr, "Reinitializing...\n")
		if err := ensureKBDir(); err != nil {
			return err
		}

		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("reinitialize: %w", err)
		}
		store.Close()

		fmt.Fprintf(os.Stderr, "Knowledge base reset complete.\n")
		return nil
	},
}

func init() {
	kbResetCmd.Flags().BoolVar(&kbResetForce, "force", false, "Skip confirmation")
	kbCmd.AddCommand(kbResetCmd)
}
