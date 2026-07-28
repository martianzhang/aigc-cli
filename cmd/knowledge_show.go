package cmd

import (
	"database/sql"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
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
		defer store.Close()

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

		fmt.Printf("ID:       %s\n", doc.ID)
		if doc.URL != "" {
			fmt.Printf("URL:      %s\n", doc.URL)
			if host := extractHost(doc.URL); host != "" {
				fmt.Printf("Source:   %s\n", host)
			}
		}
		if doc.FilePath != "" {
			fmt.Printf("File:     %s\n", doc.FilePath)
		}
		fmt.Printf("Title:    %s\n", doc.Title)
		if doc.Project != "" {
			fmt.Printf("Project:  %s\n", doc.Project)
		}
		fmt.Printf("Size:     %.1f KB\n", float64(doc.Size)/1024)
		fmt.Printf("Created:  %s\n", doc.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("Updated:  %s\n", doc.UpdatedAt.Format("2006-01-02 15:04:05"))
		fmt.Println()

		// Show chunks
		rows, err := store.DB().Query(
			"SELECT id, chunk_index, content, heading FROM chunks WHERE doc_id=? ORDER BY chunk_index",
			doc.ID)
		if err != nil {
			return fmt.Errorf("query chunks: %w", err)
		}
		defer rows.Close()

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
				fmt.Printf("--- Chunk %d: %s ---\n", chunkIndex, heading)
			} else {
				fmt.Printf("--- Chunk %d ---\n", chunkIndex)
			}
			fmt.Println(content)
			fmt.Println()
		}

		return rows.Err()
	},
}

// findDocByPrefix finds a document by matching the ID prefix.
func findDocByPrefix(store *knowledge.Store, prefix string) (*knowledge.Document, error) {
	rows, err := store.DB().Query(
		"SELECT id FROM documents WHERE id LIKE ? || '%'", prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fullID string
	if rows.Next() {
		if err := rows.Scan(&fullID); err != nil {
			return nil, err
		}
	}
	if rows.Next() {
		return nil, fmt.Errorf("ambiguous prefix: multiple documents match %q", prefix)
	}
	if fullID == "" {
		return nil, nil
	}
	return store.GetDocument(fullID)
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
		if len(d.ID) >= 12 && d.ID[:12] == docID || d.ID == docID {
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

	fmt.Printf("Title: %s\nID: %s\n", match.Title, match.ID[:12])
	if match.URL != "" {
		fmt.Printf("URL: %s\n", match.URL)
	}
	if match.FilePath != "" {
		fmt.Printf("File: %s\n", match.FilePath)
	}
	fmt.Printf("Size: %d bytes\n\n", len(plaintext))
	fmt.Println(string(plaintext))

	return nil
}

// ensure sql is used
var _ = sql.ErrNoRows
