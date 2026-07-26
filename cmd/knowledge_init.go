package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/vault"
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
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		count, _ := store.CountDocuments()
		fmt.Fprintf(os.Stderr, "Knowledge base initialized at %s\n", kbBaseDir)
		if count > 0 {
			fmt.Fprintf(os.Stderr, "Existing documents: %d\n", count)
		}

		docsDir := kbBaseDir + "/docs"
		if err := os.MkdirAll(docsDir, 0755); err != nil {
			return fmt.Errorf("create docs dir: %w", err)
		}

		// Download embedding model to shared models dir
		modelsDir := filepath.Join(configDir(), "models")
		fmt.Fprintf(os.Stderr, "Downloading embedding model...\n")
		if err := knowledge.InitEmbedModel(modelsDir, kbInitForceModel); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: model download failed: %v\n", err)
			fmt.Fprintf(os.Stderr, "  The knowledge base will work without it, but search quality\n")
			fmt.Fprintf(os.Stderr, "  will be limited until 'kb init' completes successfully.\n")
			fmt.Fprintf(os.Stderr, "  Retry with: aigc-cli kb init --force\n")
		} else {
			fmt.Fprintf(os.Stderr, "Embedding model ready.\n")
		}

		// Initialize vault keys
		if !vault.IdentityExists() {
			fmt.Fprintf(os.Stderr, "Generating vault key...\n")
			pubKey, err := vault.InitIdentity()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: vault key generation failed: %v\n", err)
				fmt.Fprintf(os.Stderr, "  Vault features (--vault) will not be available.\n")
			} else {
				fmt.Fprintf(os.Stderr, "Vault key generated. Public key: %s\n", pubKey)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Vault key already exists.\n")
		}

		return nil
	},
}

func init() {
	kbInitCmd.Flags().BoolVar(&kbInitForceModel, "force", false, "Re-download models even if they exist")
}
