package knowledge

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/search"
)

func fetchSearchResults(store *knowledge.Store, cmd *cobra.Command, results []search.Result, query, project string, verbose bool) error {
	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder := knowledge.NewHashEmbedder(384)

	for _, sr := range results {
		if verbose {
			fmt.Fprintf(os.Stderr, "  Fetching: %s\n", sr.URL)
		}
		time.Sleep(500 * time.Millisecond)

		fetchResult, fetchErr := knowledge.FetchURL(sr.URL)
		if fetchErr != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Skip: %v\n", fetchErr)
			}
			continue
		}

		docID := knowledge.Checksum(fetchResult.Content)
		existing, _ := store.GetDocument(docID)
		if existing != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Already in KB\n")
			}
			if content := tryReadDoc(docID); content != "" {
				outputDoc(docID, fetchResult.Title, fetchResult.URL, content)
			} else {
				outputDoc(docID, fetchResult.Title, fetchResult.URL, fetchResult.Content)
			}
			continue
		}

		if shouldAutoSave() {
			doc := &knowledge.Document{
				ID: docID, URL: fetchResult.URL, Title: fetchResult.Title,
				Project: project, Size: fetchResult.Size, Checksum: docID,
			}
			if err := store.SaveDocument(doc); err != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "    Error saving: %v\n", err)
				}
			} else {
				knowledge.SaveDocFile(kbBaseDir, project, docID, fetchResult.Title, fetchResult.Content)

				rawChunks := chunker.Chunk(fetchResult.Content)
				embeddings := make([]knowledge.Embedding, len(rawChunks))
				for i, c := range rawChunks {
					emb, err := embedder.Embed(c.Content)
					if err != nil {
						continue
					}
					embeddings[i] = emb
				}
				if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
					if verbose {
						fmt.Fprintf(os.Stderr, "    Error saving chunks: %v\n", err)
					}
				}
				if verbose {
					fmt.Fprintf(os.Stderr, "    Saved: %s\n", fetchResult.Title)
				}
			}
		}
		outputDoc(docID, fetchResult.Title, fetchResult.URL, fetchResult.Content)
	}
	return nil
}

func outputDoc(docID, title, url, content string) {
	fmt.Println("---")
	fmt.Printf("id: %s\n", docID[:12])
	fmt.Printf("title: %s\n", title)
	fmt.Printf("url: %s\n", url)
	if host := extractHost(url); host != "" {
		fmt.Printf("source: %s\n", host)
	}
	fmt.Println(content)
}

// extractHost returns the hostname from a URL, or empty string.
func extractHost(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
}

func tryReadDoc(docID string) string {
	docsDir := filepath.Join(kbBaseDir, "docs")
	var found string
	filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), docID[:12]) {
			found = path
			return fmt.Errorf("stop")
		}
		return nil
	})
	if found == "" {
		return ""
	}
	data, err := os.ReadFile(found)
	if err != nil {
		return ""
	}
	return string(data)
}

func outputLocalResults(results knowledge.SearchResults, verbose bool) {
	// Aggregate by document
	type docRes struct {
		docID  string
		title  string
		source string
		score  float64
		nchunk int
	}
	docMap := make(map[string]*docRes)
	for _, r := range results {
		d, ok := docMap[r.Document.ID]
		if !ok {
			source := r.Document.URL
			if source == "" {
				source = r.Document.FilePath
			}
			docMap[r.Document.ID] = &docRes{
				docID: r.Document.ID[:12], title: r.Document.Title,
				source: source, score: r.Score,
			}
			d = docMap[r.Document.ID]
		}
		if r.Score > d.score {
			d.score = r.Score
		}
		d.nchunk++
	}

	docs := make([]*docRes, 0, len(docMap))
	for _, d := range docMap {
		docs = append(docs, d)
	}
	for i := 0; i < len(docs); i++ {
		for j := i + 1; j < len(docs); j++ {
			if docs[j].score > docs[i].score {
				docs[i], docs[j] = docs[j], docs[i]
			}
		}
	}

	if len(docs) == 0 {
		return
	}

	if !verbose {
		return
	}

	fmt.Printf("From knowledge base:\n\n")
	for i, d := range docs {
		fmt.Printf("[%d] %s\n", i+1, d.title)
		fmt.Printf("    id: %s | source: %s | score: %.4f (%d chunk(s))\n", d.docID, d.source, d.score, d.nchunk)
	}
	fmt.Println()
}
