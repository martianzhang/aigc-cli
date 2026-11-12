package knowledge

import (
	"database/sql"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/ui"
	"github.com/martianzhang/aigc-cli/internal/vault"
	"github.com/spf13/cobra"
)

// kbShowCmd shows document details and its chunks.
var kbShowCmd = &cobra.Command{
	Use:   "show <doc-id>",
	Short: "Show document details and chunks",
	Long: `Show the full details and content of a document.

The doc-id is the first 12 characters of the document hash,
as shown in 'kb list'.`,
	Example: `  aigc-cli kb show a1b2c3d4e5f6
  aigc-cli kb show a1b2c3d4e5f6 --vault`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultEnabled {
			return showVault(args[0])
		}
		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer func() { _ = store.Close() }()

		// Try as full ID, then as prefix
		docID := args[0]
		doc, err := store.GetDocument(docID)
		if err != nil {
			return fmt.Errorf("get doc: %w", err)
		}
		if doc == nil {
			// Try prefix match
			doc, err = findDocByPrefix(store, docID)
			if err != nil {
				return err
			}
			if doc == nil {
				return fmt.Errorf("document not found: %s", docID)
			}
		}

		ui.Printf("ID:       %s\n", doc.ID)
		if doc.URL != "" {
			ui.Printf("URL:      %s\n", doc.URL)
			if host := extractHost(doc.URL); host != "" {
				ui.Printf("Source:   %s\n", host)
			}
		}
		if doc.FilePath != "" {
			ui.Printf("File:     %s\n", doc.FilePath)
		}
		ui.Printf("Title:    %s\n", doc.Title)
		if doc.Project != "" {
			ui.Printf("Project:  %s\n", doc.Project)
		}
		ui.Printf("Size:     %.1f KB\n", float64(doc.Size)/1024)
		ui.Printf("Created:  %s\n", doc.CreatedAt.Format("2006-01-02 15:04:05"))
		ui.Printf("Updated:  %s\n", doc.UpdatedAt.Format("2006-01-02 15:04:05"))
		ui.Println()

		// Show chunks
		rows, err := store.DB().Query(
			"SELECT id, chunk_index, content, heading FROM chunks WHERE doc_id=? ORDER BY chunk_index",
			doc.ID)
		if err != nil {
			return fmt.Errorf("query chunks: %w", err)
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var (
				chunkID    int64
				chunkIndex int
				content    string
				heading    string
			)
			if err := rows.Scan(&chunkID, &chunkIndex, &content, &heading); err != nil {
				return fmt.Errorf("scan chunk: %w", err)
			}
			if heading != "" {
				ui.Printf("--- Chunk %d: %s ---\n", chunkIndex, heading)
			} else {
				ui.Printf("--- Chunk %d ---\n", chunkIndex)
			}
			ui.Println(content)
			ui.Println()
		}

		return rows.Err()
	},
}

// findDocByPrefix finds a document by matching the ID prefix.
func findDocByPrefix(store *knowledge.Store, prefix string) (*knowledge.Document, error) {
	return store.FindByIDPrefix(prefix)
}

func showVault(docID string) error {
	v, err := vault.Open(vaultBaseDir)
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}

	// Find by prefix
	docs, err := v.List()
	if err != nil {
		return fmt.Errorf("list: %w", err)
	}
	var match *vault.VaultDoc
	for _, d := range docs {
		if len(d.ID) >= 12 && knowledge.ShortID(d.ID) == docID || d.ID == docID {
			if match != nil {
				return fmt.Errorf("ambiguous prefix: multiple documents match %q", docID)
			}
			match = &d
		}
	}
	if match == nil {
		return fmt.Errorf("document not found: %s", docID)
	}

	_, plaintext, err := v.Load(match.ID)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}

	ui.Printf("Title: %s\nID: %s\n", match.Title, knowledge.ShortID(match.ID))
	if match.URL != "" {
		ui.Printf("URL: %s\n", match.URL)
	}
	if match.FilePath != "" {
		ui.Printf("File: %s\n", match.FilePath)
	}
	ui.Printf("Size: %d bytes\n\n", len(plaintext))
	ui.Println(string(plaintext))

	return nil
}

// ensure sql is used
var _ = sql.ErrNoRows
