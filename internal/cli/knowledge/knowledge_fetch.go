package knowledge

import (
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/vault"
	"github.com/spf13/cobra"
)

var kbFetchCmd = &cobra.Command{
	Use:   "fetch <url> [url ...]",
	Short: "Fetch URL(s) and add to the knowledge base",
	Long: `Fetch web page(s), extract main content, and store.

Use --vault to encrypt and store in the vault instead.`,
	Example: `  aigc-cli kb fetch https://go.dev/doc/effective_go
  aigc-cli kb fetch https://example.com/a https://example.com/b
  aigc-cli kb fetch https://example.com/private --vault`,
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultEnabled {
			return fetchToVault(args)
		}
		return fetchToKB(cmd, args)
	},
}

func fetchToKB(cmd *cobra.Command, args []string) error {
	store, err := openKBStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder, err := options.BuildKBEmbedder()
	if err != nil {
		return fmt.Errorf("embedder: %w", err)
	}
	project := detectProject(cmd)

	for _, url := range args {
		fmt.Fprintf(os.Stderr, "  Fetching: %s\n", url)

		result, err := knowledge.FetchURL(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			continue
		}

		checksum := knowledge.Checksum(result.Content)
		docID := checksum

		existing, err := store.GetDocument(docID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error checking: %v\n", err)
			continue
		}
		if existing != nil {
			fmt.Fprintf(os.Stderr, "  Skipped (already exists): %s\n", result.Title)
			continue
		}

		doc := &knowledge.Document{
			ID: docID, URL: result.URL, Title: result.Title,
			Project: project, Size: result.Size, Checksum: checksum,
		}
		if err := store.SaveDocument(doc); err != nil {
			fmt.Fprintf(os.Stderr, "  Error saving doc: %v\n", err)
			continue
		}
		knowledge.SaveDocFile(kbBaseDir, project, docID, result.Title, result.Content)

		rawChunks := chunker.Chunk(result.Content)
		contents := make([]string, len(rawChunks))
		for i, c := range rawChunks {
			contents[i] = c.Content
		}
		embeddings, err := knowledge.EmbedAll(embedder, contents)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error embedding: %v\n", err)
			continue
		}
		if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
			fmt.Fprintf(os.Stderr, "  Error saving chunks: %v\n", err)
			continue
		}

		fmt.Fprintf(os.Stderr, "  Added: %s (%d chunks, %s)\n",
			result.Title, len(rawChunks), result.URL)
	}
	return nil
}

// fetchToVault fetches URLs and stores them encrypted in the vault.
func fetchToVault(args []string) error {
	store, err := openKBStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer store.Close()

	v, err := vault.Open(vaultBaseDir)
	if err != nil {
		return fmt.Errorf("open vault: %w", err)
	}

	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder, err := options.BuildKBEmbedder()
	if err != nil {
		return fmt.Errorf("embedder: %w", err)
	}

	for _, url := range args {
		fmt.Fprintf(os.Stderr, "  Fetching: %s\n", url)

		result, err := knowledge.FetchURL(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error: %v\n", err)
			continue
		}

		docID := knowledge.Checksum(result.Content)
		existing, _ := store.GetDocument(docID)
		if existing != nil {
			fmt.Fprintf(os.Stderr, "  Skipped (already exists): %s\n", result.Title)
			continue
		}

		// Save metadata to SQLite (no plaintext)
		doc := &knowledge.Document{
			ID: docID, URL: result.URL, Title: result.Title,
			IsVault: true, Size: result.Size, Checksum: docID,
		}
		if err := store.SaveDocument(doc); err != nil {
			fmt.Fprintf(os.Stderr, "  Error saving doc: %v\n", err)
			continue
		}

		// Store vectors for search
		rawChunks := chunker.Chunk(result.Content)
		contents := make([]string, len(rawChunks))
		for i, c := range rawChunks {
			contents[i] = c.Content
		}
		embeddings, err := knowledge.EmbedAll(embedder, contents)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error embedding: %v\n", err)
			continue
		}
		if err := store.SaveVaultEmbeddings(docID, embeddings); err != nil {
			fmt.Fprintf(os.Stderr, "  Error saving vectors: %v\n", err)
		}

		// Encrypt content to vault file
		vaultDoc := &vault.VaultDoc{
			ID: docID, URL: result.URL, Title: result.Title,
			Size: result.Size,
		}
		if err := v.Save(vaultDoc, []byte(result.Content)); err != nil {
			fmt.Fprintf(os.Stderr, "  Error encrypting: %v\n", err)
			continue
		}

		fmt.Fprintf(os.Stderr, "  Added to vault: %s\n", result.Title)
	}
	return nil
}
