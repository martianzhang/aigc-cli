package agent

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
)

func KbAdd(kbDir, argsJSON string) string {
	var args struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.FilePath == "" {
		return "Error: file_path is required"
	}

	if err := os.MkdirAll(kbDir, 0755); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	store, err := knowledge.OpenStore(kbDir, 384, nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer store.Close()

	title, content, err := knowledge.LoadFile(args.FilePath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	checksum := knowledge.Checksum(content)
	docID := checksum

	existing, _ := store.GetDocument(docID)
	if existing != nil {
		return fmt.Sprintf("Already in KB: %s", title)
	}

	doc := &knowledge.Document{
		ID:       docID,
		FilePath: args.FilePath,
		Title:    title,
		Size:     int64(len(content)),
		Checksum: checksum,
	}
	if err := store.SaveDocument(doc); err != nil {
		return fmt.Sprintf("Error saving: %v", err)
	}

	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder := knowledge.NewHashEmbedder(384)
	rawChunks := chunker.Chunk(content)
	embeddings := make([]knowledge.Embedding, len(rawChunks))
	for i, c := range rawChunks {
		emb, err := embedder.Embed(c.Content)
		if err != nil {
			continue
		}
		embeddings[i] = emb
	}
	if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
		return fmt.Sprintf("Error saving chunks: %v", err)
	}

	return fmt.Sprintf("Added %q to the knowledge base (%d chunks).", title, len(rawChunks))
}

func KbFetch(kbDir, argsJSON string) string {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.URL == "" {
		return "Error: url is required"
	}

	if err := os.MkdirAll(kbDir, 0755); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	store, err := knowledge.OpenStore(kbDir, 384, nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer store.Close()

	result, err := knowledge.FetchURL(args.URL)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	docID := knowledge.Checksum(result.Content)
	existing, _ := store.GetDocument(docID)
	if existing != nil {
		return fmt.Sprintf("Already in KB: %s", result.Title)
	}

	doc := &knowledge.Document{
		ID:       docID,
		URL:      result.URL,
		Title:    result.Title,
		Size:     result.Size,
		Checksum: docID,
	}
	if err := store.SaveDocument(doc); err != nil {
		return fmt.Sprintf("Error saving: %v", err)
	}

	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder := knowledge.NewHashEmbedder(384)
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
		return fmt.Sprintf("Error saving chunks: %v", err)
	}

	snippet := result.Content
	if len(snippet) > 500 {
		snippet = snippet[:500] + "..."
	}
	return fmt.Sprintf("Added %s to KB.\nTitle: %s\n\nPreview:\n%s", result.URL, result.Title, snippet)
}
