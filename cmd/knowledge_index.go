package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/spf13/cobra"
)

var kbIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Re-index all documents from docs/ directory",
	Long: `Scan the docs/ directory, re-chunk, re-embed, and re-index
all documents. Use this after updating the chunking strategy or
embedding model, or after manually copying files into docs/.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
		embedder := knowledge.NewHashEmbedder(384)

		docsDir := filepath.Join(kbBaseDir, "docs")
		info, err := os.Stat(docsDir)
		if err != nil || !info.IsDir() {
			fmt.Fprintln(os.Stderr, "docs/ directory not found. Add documents with 'kb add' or 'kb fetch' first.")
			return nil
		}

		type docFile struct {
			path    string
			project string
		}
		var files []docFile

		err = filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".md") {
				return nil
			}
			rel, _ := filepath.Rel(docsDir, path)
			parts := strings.SplitN(rel, string(filepath.Separator), 2)
			project := ""
			if len(parts) == 2 {
				project = parts[0]
			}
			if project == "global" {
				project = ""
			}
			files = append(files, docFile{path: path, project: project})
			return nil
		})
		if err != nil {
			return fmt.Errorf("walk docs/: %w", err)
		}

		if len(files) == 0 {
			fmt.Fprintln(os.Stderr, "No .md files found in docs/.")
			return nil
		}

		for _, f := range files {
			data, err := os.ReadFile(f.path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error reading %s: %v\n", f.path, err)
				continue
			}
			content := string(data)
			if len(content) == 0 {
				continue
			}

			docID := knowledge.Checksum(content)
			title := filepath.Base(f.path)
			if idx := strings.Index(title, "-"); idx > 0 {
				title = strings.TrimSuffix(title[idx+1:], ".md")
			}

			doc := &knowledge.Document{
				ID:       docID,
				Title:    title,
				Project:  f.project,
				Size:     int64(len(content)),
				Checksum: docID,
			}
			if err := store.SaveDocument(doc); err != nil {
				fmt.Fprintf(os.Stderr, "  Error saving doc %s: %v\n", f.path, err)
				continue
			}

			rawChunks := chunker.Chunk(content)
			embeddings := make([]knowledge.Embedding, len(rawChunks))
			for i, c := range rawChunks {
				emb, err := embedder.Embed(c.Content)
				if err != nil {
					return fmt.Errorf("embed: %w", err)
				}
				embeddings[i] = emb
			}
			if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
				return fmt.Errorf("save chunks: %w", err)
			}

			fmt.Fprintf(os.Stderr, "  Indexed: %s (%d chunks)\n", filepath.Base(f.path), len(rawChunks))
		}

		return nil
	},
}
