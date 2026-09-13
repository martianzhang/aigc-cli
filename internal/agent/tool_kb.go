package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
)

func KbFind(kbDir, argsJSON string) string {
	var args struct {
		Query string  `json:"query"`
		Limit float64 `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Query == "" {
		return "Error: query is required"
	}
	limit := 10
	if args.Limit > 0 {
		limit = int(args.Limit)
	}

	if err := os.MkdirAll(kbDir, 0755); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	store, err := knowledge.OpenStore(kbDir, 384, nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer store.Close()

	results, err := store.Search(args.Query, limit*3, "")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if len(results) == 0 {
		return "No results found in the knowledge base."
	}

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
	sort.Slice(docs, func(i, j int) bool { return docs[i].score > docs[j].score })
	if limit > 0 && len(docs) > limit {
		docs = docs[:limit]
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Found %d matching document(s):\n\n", len(docs))
	for i, d := range docs {
		fmt.Fprintf(&out, "[%d] %s\n", i+1, d.title)
		fmt.Fprintf(&out, "    ID: %s | Source: %s | Score: %.4f (%d chunk(s))\n", d.docID, d.source, d.score, d.nchunk)
		fmt.Fprintf(&out, "    Use: kb_show %s\n", d.docID)
	}
	return out.String()
}

func KbSearch(kbDir, argsJSON string) string {
	var args struct {
		Query    string `json:"query"`
		Provider string `json:"provider"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Query == "" {
		return "Error: query is required"
	}

	if err := os.MkdirAll(kbDir, 0755); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	store, err := knowledge.OpenStore(kbDir, 384, nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer store.Close()

	// Search via DuckDuckGo
	urls, err := knowledge.DDGSearchURLs(args.Query)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	if len(urls) == 0 {
		return fmt.Sprintf("No search results found for %q.", args.Query)
	}

	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder := knowledge.NewHashEmbedder(384)
	var out strings.Builder
	fmt.Fprintf(&out, "Searched for %q, saved %d result(s):\n", args.Query, len(urls))

	for _, rawURL := range urls {
		result, err := knowledge.FetchURL(rawURL)
		if err != nil {
			fmt.Fprintf(&out, "\n  \u274c %s", rawURL)
			continue
		}

		docID := knowledge.Checksum(result.Content)
		existing, _ := store.GetDocument(docID)
		if existing != nil {
			fmt.Fprintf(&out, "\n  \u2713 %s (already saved)", result.Title)
			continue
		}

		doc := &knowledge.Document{
			ID:       docID,
			URL:      result.URL,
			Title:    result.Title,
			Size:     result.Size,
			Checksum: docID,
		}
		if err := store.SaveDocument(doc); err != nil {
			fmt.Fprintf(&out, "\n  \u274c %s: %v", result.Title, err)
			continue
		}

		rawChunks := chunker.Chunk(result.Content)
		embeddings := make([]knowledge.Embedding, len(rawChunks))
		for i, c := range rawChunks {
			emb, err := embedder.Embed(c.Content)
			if err != nil {
				continue
			}
			embeddings[i] = emb
		}
		if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
			fmt.Fprintf(&out, "\n  \u274c %s: %v", result.Title, err)
			continue
		}
		fmt.Fprintf(&out, "\n  \u2713 %s (%s)", result.Title, result.URL)
	}
	return out.String()
}
