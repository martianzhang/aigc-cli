package ideas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
)

// fakeEmbedder maps text to a 2-d vector by keyword, so ranking is deterministic.
type fakeEmbedder struct{}

func (fakeEmbedder) Dim() int     { return 2 }
func (fakeEmbedder) Name() string { return "fake:test" }
func (fakeEmbedder) Embed(text string) (knowledge.Embedding, error) {
	low := strings.ToLower(text)
	switch {
	case strings.Contains(low, "cat"):
		return knowledge.Embedding{1, 0}, nil
	case strings.Contains(low, "dog"):
		return knowledge.Embedding{0, 1}, nil
	default:
		return knowledge.Embedding{0.5, 0.5}, nil
	}
}

func TestEmbeddingCacheRoundtrip(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "ideas.json")
	entries := []IdeaEntry{{Prompt: "a cat"}, {Prompt: "a dog"}}

	vectors, err := LoadOrBuildEmbeddings(entries, fakeEmbedder{}, dataPath, nil)
	if err != nil {
		t.Fatalf("LoadOrBuildEmbeddings: %v", err)
	}
	if len(vectors) != 2 || len(vectors[0]) != 2 {
		t.Fatalf("got %d vectors of dim %d, want 2 of dim 2", len(vectors), len(vectors[0]))
	}

	path, err := EmbeddingCachePath(dataPath, "fake:test", 2)
	if err != nil {
		t.Fatalf("EmbeddingCachePath: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not written: %v", err)
	}

	if _, ok := loadEmbeddingCache(path, "fake:test", 2, 2, datasetHash(entries)); !ok {
		t.Error("loadEmbeddingCache() = miss, want hit for matching dataset")
	}
	if _, ok := loadEmbeddingCache(path, "fake:test", 2, 2, datasetHash(entries[:1])); ok {
		t.Error("loadEmbeddingCache() = hit, want miss after the dataset changes")
	}
	if _, ok := loadEmbeddingCache(path, "other:model", 2, 2, datasetHash(entries)); ok {
		t.Error("loadEmbeddingCache() = hit, want miss for a different model")
	}
}

func TestSemanticEntriesRanksByCosine(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "ideas.json")
	entries := []IdeaEntry{{Prompt: "a cat"}, {Prompt: "a dog"}}
	vectors, err := LoadOrBuildEmbeddings(entries, fakeEmbedder{}, dataPath, nil)
	if err != nil {
		t.Fatalf("LoadOrBuildEmbeddings: %v", err)
	}

	got, err := SemanticEntries(entries, vectors, fakeEmbedder{}, "cat", 0)
	if err != nil {
		t.Fatalf("SemanticEntries: %v", err)
	}
	if len(got) != 2 || got[0].Prompt != "a cat" {
		t.Errorf("SemanticEntries(cat) = %v, want 'a cat' first", got)
	}
}

func TestEmbeddingCachePathSanitizesModel(t *testing.T) {
	path, err := EmbeddingCachePath(filepath.Join(t.TempDir(), "ideas.json"), "api:embedding/gemma 2", 768)
	if err != nil {
		t.Fatalf("EmbeddingCachePath: %v", err)
	}
	if strings.ContainsAny(filepath.Base(path), ":/ ") {
		t.Errorf("cache filename %q contains unsafe characters", filepath.Base(path))
	}
}
