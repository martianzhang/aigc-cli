// Package knowledge provides a local knowledge base with full-text and
// semantic search, web fetch pipeline, search provider routing, and
// vault encryption.
package knowledge

import "time"

// Document represents a source document in the knowledge base.
type Document struct {
	ID        string    `json:"id"` // SHA256 of content
	URL       string    `json:"url,omitempty"`
	FilePath  string    `json:"filepath,omitempty"`
	Title     string    `json:"title,omitempty"`
	Project   string    `json:"project,omitempty"`
	IsVault   bool      `json:"is_vault,omitempty"`
	Size      int64     `json:"size"`
	Checksum  string    `json:"checksum"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Chunk represents a single chunk of a document.
type Chunk struct {
	ID        int64     `json:"id"`
	DocID     string    `json:"doc_id"`
	Index     int       `json:"index"`
	Content   string    `json:"content"`
	Heading   string    `json:"heading,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Embedding is a vector whose length is the embedder's dimension. It is
// variable because the backend is pluggable (hash=384, ONNX=384, API models
// vary, e.g. embeddinggemma-2=768).
type Embedding []float32

// SearchResult holds one hit from a search.
type SearchResult struct {
	Chunk    Chunk    `json:"chunk"`
	Document Document `json:"document"`
	Score    float64  `json:"score"`
}

// SearchResults is a sortable slice of search results.
type SearchResults []SearchResult

func (s SearchResults) Len() int           { return len(s) }
func (s SearchResults) Less(i, j int) bool { return s[i].Score < s[j].Score }
func (s SearchResults) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// Embedder creates document embeddings from text.
type Embedder interface {
	Embed(text string) (Embedding, error)
	Dim() int
}

// BatchEmbedder is implemented by embedders that can embed several texts in a
// single round trip. EmbedAll uses it when available.
type BatchEmbedder interface {
	EmbedBatch(texts []string) ([]Embedding, error)
}

// NamedEmbedder reports a stable identifier (type + model + dim) used to detect
// a changed backend so a stale index can be flagged for re-indexing.
type NamedEmbedder interface {
	Name() string
}

// EmbedAll embeds texts in one batch call when the embedder supports it, and
// falls back to per-text calls otherwise.
func EmbedAll(e Embedder, texts []string) ([]Embedding, error) {
	if b, ok := e.(BatchEmbedder); ok {
		return b.EmbedBatch(texts)
	}
	out := make([]Embedding, len(texts))
	for i, t := range texts {
		emb, err := e.Embed(t)
		if err != nil {
			return nil, err
		}
		out[i] = emb
	}
	return out, nil
}

// ChunkOptions configures the chunking strategy.
type ChunkOptions struct {
	MaxSize   int  // max characters per chunk
	Overlap   int  // overlap between chunks
	SplitCode bool // attempt to split code blocks
}

// DefaultChunkOptions returns sensible defaults.
func DefaultChunkOptions() ChunkOptions {
	return ChunkOptions{
		MaxSize:   2048,
		Overlap:   128,
		SplitCode: true,
	}
}

// KBConfig configures the knowledge base location.
type KBConfig struct {
	BaseDir string // root directory for KB data
}
