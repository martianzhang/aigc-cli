package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/vault"
	"github.com/spf13/cobra"
)

var kbAddRecursive bool

// kbAddCmd adds a local file or directory to the knowledge base.
var kbAddCmd = &cobra.Command{
	Use:   "add <path> [path ...]",
	Short: "Add local file(s) to the knowledge base",
	Long: `Add one or more local files to the knowledge base.
Supports: .md, .txt, .go, .py, .rs, .json, .yaml, .html and more.
For PDF: convert to text with 'aigc-cli ocr scan' first.
For DOCX: convert to markdown with officecli first.

Use --recursive/-r to add all supported files in a directory.`,
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if vaultEnabled {
			return addToVault(args)
		}
		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
		embedder := knowledge.NewHashEmbedder(384)

		var files []string
		for _, arg := range args {
			info, err := os.Stat(arg)
			if err != nil {
				return fmt.Errorf("stat %s: %w", arg, err)
			}
			if info.IsDir() {
				if !kbAddRecursive {
					return fmt.Errorf("%s is a directory; use --recursive/-r to add directory contents", arg)
				}
				dirFiles, err := knowledge.LoadDir(arg)
				if err != nil {
					return fmt.Errorf("list %s: %w", arg, err)
				}
				files = append(files, dirFiles...)
			} else {
				files = append(files, arg)
			}
		}

		if len(files) == 0 {
			fmt.Fprintln(os.Stderr, "No supported files found.")
			return nil
		}

		// Resolve project scope and loaders
		project := detectProject(cmd)
		loaders := resolveLoaders()

		for _, f := range files {
			if err := processAndStoreFile(store, chunker, embedder, f, project, loaders); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s: %v\n", f, err)
			}
		}

		return nil
	},
}

func init() {
	kbAddCmd.Flags().BoolVarP(&kbAddRecursive, "recursive", "r", false, "Recursively add all supported files in directories")
}

// processAndStoreFile reads a file, chunks it, embeds it, and stores it.
// loaders is an optional map of extension→command for external loaders.
func processAndStoreFile(store *knowledge.Store, chunker *knowledge.Chunker, embedder *knowledge.HashEmbedder, filePath, project string, loaders map[string]string) error {
	title, content, err := knowledge.LoadFile(filePath)
	if err != nil {
		// Try external loader for unsupported file types
		ext := strings.ToLower(filepath.Ext(filePath))
		if cmd, ok := loaders[ext]; ok {
			title, content, err = knowledge.RunExternalLoader(cmd, filePath)
		}
		if err != nil {
			return fmt.Errorf("load: %w", err)
		}
	}

	checksum := knowledge.Checksum(content)
	docID := checksum

	// Check if already indexed with same checksum
	existing, err := store.GetDocument(docID)
	if err != nil {
		return fmt.Errorf("check existing: %w", err)
	}
	if existing != nil && existing.Checksum == checksum {
		fmt.Fprintf(os.Stderr, "  Skipped (unchanged): %s\n", filePath)
		return nil
	}

	doc := &knowledge.Document{
		ID:       docID,
		FilePath: filePath,
		Title:    title,
		Project:  project,
		Size:     int64(len(content)),
		Checksum: checksum,
	}
	if err := store.SaveDocument(doc); err != nil {
		return fmt.Errorf("save doc: %w", err)
	}

	// Save raw content to docs/<sha1>.md
	knowledge.SaveDocFile(kbBaseDir, project, docID, title, content)

	// Chunk
	rawChunks := chunker.Chunk(content)

	// Embed each chunk
	embeddings := make([]knowledge.Embedding, len(rawChunks))
	for i, c := range rawChunks {
		emb, err := embedder.Embed(c.Content)
		if err != nil {
			return fmt.Errorf("embed chunk %d: %w", i, err)
		}
		embeddings[i] = emb
	}

	// Store
	if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
		return fmt.Errorf("save chunks: %w", err)
	}

	fmt.Fprintf(os.Stderr, "  Added: %s (%d chunks)\n", filePath, len(rawChunks))
	return nil
}

// addToVault encrypts files and adds them to the vault.
func addToVault(args []string) error {
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
	embedder := knowledge.NewHashEmbedder(384)

	for _, arg := range args {
		data, err := os.ReadFile(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s: %v\n", arg, err)
			continue
		}
		content := string(data)
		title := filepath.Base(arg)
		docID := knowledge.Checksum(content)

		// Save document metadata to SQLite (no plaintext content)
		doc := &knowledge.Document{
			ID:       docID,
			FilePath: arg,
			Title:    title,
			IsVault:  true,
			Size:     int64(len(data)),
			Checksum: docID,
		}
		if err := store.SaveDocument(doc); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving doc: %s: %v\n", arg, err)
			continue
		}

		// Chunk, embed, and store vectors only (no plaintext for vault)
		rawChunks := chunker.Chunk(content)
		embeddings := make([]knowledge.Embedding, len(rawChunks))
		for i, c := range rawChunks {
			emb, err := embedder.Embed(c.Content)
			if err != nil {
				continue
			}
			embeddings[i] = emb
		}
		if err := store.SaveVaultEmbeddings(docID, embeddings); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving vectors: %s: %v\n", arg, err)
		}

		// Encrypt content to vault file
		vaultDoc := &vault.VaultDoc{
			ID:       docID,
			FilePath: arg,
			Title:    title,
			Size:     int64(len(data)),
		}
		if err := v.Save(vaultDoc, data); err != nil {
			fmt.Fprintf(os.Stderr, "Error encrypting: %s: %v\n", arg, err)
			continue
		}

		fmt.Fprintf(os.Stderr, "  Added to vault: %s\n", title)
	}
	return nil
}
