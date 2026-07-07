package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/apimart-cli/internal/types"
)

// bm25CacheData is the serializable form of bm25Index for disk caching.
// Only computed statistics are cached; docTexts is cheap to rebuild from entries.
type bm25CacheData struct {
	AvgDocLen float64
	DocCount  int
	Idf       map[string]float64
	DocTokens [][]string
	DocSet    []map[string]int
	Hash      []byte // SHA256 of source ideas.json, for consistency check
}

func init() {
	gob.Register(bm25CacheData{})
}

// --- Path resolution ---

// ideasDir returns the default directory for ideas data.
// Uses ~/.config/aigc-cli/ (new), with fallback to ~/.config/apimart/ (backward compat).
func ideasDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	newDir := filepath.Join(home, ".config", "aigc-cli")
	oldDir := filepath.Join(home, ".config", "apimart")
	// Prefer new path; fall back to old path for existing users
	if _, err := os.Stat(newDir); err == nil {
		return newDir, nil
	}
	if _, err := os.Stat(oldDir); err == nil {
		return oldDir, nil
	}
	return newDir, nil
}

// resolveIdeasDataPath returns the path to an existing external ideas.json.
// Order: config ideas.data_path → ~/.config/aigc-cli/ideas.json → fallback to ~/.config/apimart/ → empty.
func resolveIdeasDataPath(cfg *types.Config) string {
	if cfg != nil && cfg.Ideas != nil && cfg.Ideas.DataPath != "" {
		return cfg.Ideas.DataPath
	}
	dir, err := ideasDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(dir, "ideas.json")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// ideasDataSavePath returns the path where ideas.json should be saved.
// Order: config ideas.data_path → ~/.config/aigc-cli/ideas.json.
// Unlike resolveIdeasDataPath, this does NOT check file existence.
func ideasDataSavePath(cfg *types.Config) string {
	if cfg != nil && cfg.Ideas != nil && cfg.Ideas.DataPath != "" {
		return cfg.Ideas.DataPath
	}
	dir, err := ideasDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ideas.json")
}

// resolveIndexPath returns the path to the cache index file.
// Order: config ideas.index_path → ~/.config/aigc-cli/ideas.index.
func resolveIndexPath(cfg *types.Config) string {
	if cfg != nil && cfg.Ideas != nil && cfg.Ideas.IndexPath != "" {
		return cfg.Ideas.IndexPath
	}
	dir, err := ideasDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ideas.index")
}

// --- Cache enablement ---

// isIdeasCacheEnabled returns true if the ideas cache is enabled.
// Cache is ON by default (no config needed); set cache_enabled: false to disable.
func isIdeasCacheEnabled(cfg *types.Config) bool {
	if cfg == nil || cfg.Ideas == nil {
		return true // enabled by default
	}
	return cfg.Ideas.CacheEnabled
}

// --- Hashing ---

// computeHash returns the SHA256 checksum of data.
func computeHash(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// --- Cache load/save ---

// loadCachedIndex loads and verifies a cached BM25 index from disk.
// Returns nil if cache is disabled, file missing, hash mismatch, or corrupt.
func loadCachedIndex(cfg *types.Config, expectedHash []byte) *bm25Index {
	if !isIdeasCacheEnabled(cfg) {
		return nil
	}
	path := resolveIndexPath(cfg)
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var cd bm25CacheData
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&cd); err != nil {
		return nil
	}

	// Hash verification: the index must match the exact ideas.json content
	if !bytes.Equal(cd.Hash, expectedHash) {
		return nil
	}

	idx := &bm25Index{
		avgDocLen: cd.AvgDocLen,
		docCount:  cd.DocCount,
		idf:       cd.Idf,
		docTokens: cd.DocTokens,
		docSet:    cd.DocSet,
		docTexts:  make([]string, cd.DocCount),
	}
	// docTexts is rebuilt from entries by the caller via searchableText()
	return idx
}

// saveCachedIndex saves the BM25 index to disk cache.
// Errors are logged to stderr and silently ignored (cache is optional).
func saveCachedIndex(cfg *types.Config, idx *bm25Index, hash []byte) {
	if !isIdeasCacheEnabled(cfg) {
		return
	}
	path := resolveIndexPath(cfg)
	if path == "" {
		return
	}

	cd := &bm25CacheData{
		AvgDocLen: idx.avgDocLen,
		DocCount:  idx.docCount,
		Idf:       idx.idf,
		DocTokens: idx.docTokens,
		DocSet:    idx.docSet,
		Hash:      hash,
	}

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(cd); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to encode ideas index cache: %v\n", err)
		return
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create cache directory %s: %v\n", dir, err)
		return
	}

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save ideas index cache: %v\n", err)
	}
}
