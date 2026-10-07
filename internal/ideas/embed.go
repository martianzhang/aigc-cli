package ideas

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
)

const (
	// embedCacheMagic carries a format version; bump it to invalidate old caches.
	embedCacheMagic = "IDEAEMB2"
	embedBatchSize  = 128
	// DefaultEmbedTextMaxRunes bounds the text sent to the embedder. Full entries
	// average ~1000 runes and embedding cost scales with length, so 256 runes
	// makes the one-time build ~3x faster with negligible ranking loss.
	DefaultEmbedTextMaxRunes = 256
	// DefaultSemanticTopK bounds how many entries the semantic list contributes to RRF.
	DefaultSemanticTopK = 200
)

// EmbeddingCachePath returns the cache file for a dataset + model + dim. The
// model and dimension are part of the name so switching backends never reuses
// another backend's vectors.
func EmbeddingCachePath(dataPath, model string, dim int) (string, error) {
	if dataPath == "" {
		p, err := defaultIdeasPath()
		if err != nil {
			return "", err
		}
		dataPath = p
	}
	return filepath.Join(filepath.Dir(dataPath), fmt.Sprintf("ideas_emb_%s_%d.bin", sanitizeModel(model), dim)), nil
}

func sanitizeModel(model string) string {
	var b strings.Builder
	for _, r := range model {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

// LoadOrBuildEmbeddings returns one vector per entry, reusing the on-disk cache
// when it matches the dataset and model, and (re)building it otherwise. maxRunes
// truncates the embedded text (<=0 keeps it whole). progress, when non-nil, is
// called after each batch with (done, total).
func LoadOrBuildEmbeddings(entries []IdeaEntry, embedder knowledge.Embedder, dataPath string, maxRunes int, progress func(done, total int)) ([][]float32, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	if embedder.Dim() == 0 {
		if _, err := embedder.Embed("dimension probe"); err != nil {
			return nil, fmt.Errorf("probe embedder: %w", err)
		}
	}
	dim := embedder.Dim()
	model := embedderName(embedder)
	hash := datasetHash(entries, maxRunes)

	path, err := EmbeddingCachePath(dataPath, model, dim)
	if err != nil {
		return nil, err
	}
	if vecs, ok := loadEmbeddingCache(path, model, dim, len(entries), hash); ok {
		return vecs, nil
	}

	texts := make([]string, len(entries))
	for i, e := range entries {
		texts[i] = truncateRunes(searchableText(e), maxRunes)
	}
	vectors := make([][]float32, len(texts))
	for i := 0; i < len(texts); i += embedBatchSize {
		end := i + embedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch, err := knowledge.EmbedAll(embedder, texts[i:end])
		if err != nil {
			return nil, fmt.Errorf("embed entries %d-%d: %w", i, end, err)
		}
		for k, e := range batch {
			vectors[i+k] = []float32(e)
		}
		if progress != nil {
			progress(end, len(texts))
		}
	}
	if err := saveEmbeddingCache(path, model, dim, hash, vectors); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not cache ideas embeddings: %v\n", err)
	}
	return vectors, nil
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// SemanticEntries ranks entries by cosine similarity between the query and the
// precomputed vectors, returning the top-K entries.
func SemanticEntries(entries []IdeaEntry, vectors [][]float32, embedder knowledge.Embedder, query string, topK int) ([]IdeaEntry, error) {
	q, err := embedder.Embed(query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	type scored struct {
		idx int
		sim float64
	}
	ranked := make([]scored, len(vectors))
	for i, v := range vectors {
		ranked[i] = scored{i, cosine32(q, v)}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].sim > ranked[j].sim })
	if topK > 0 && topK < len(ranked) {
		ranked = ranked[:topK]
	}
	out := make([]IdeaEntry, len(ranked))
	for i, s := range ranked {
		out[i] = entries[s.idx]
	}
	return out, nil
}

func embedderName(e knowledge.Embedder) string {
	if n, ok := e.(knowledge.NamedEmbedder); ok {
		return n.Name()
	}
	return fmt.Sprintf("%T", e)
}

func datasetHash(entries []IdeaEntry, maxRunes int) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "maxrunes=%d\x00", maxRunes)
	for _, e := range entries {
		h.Write([]byte(truncateRunes(searchableText(e), maxRunes)))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

func cosine32(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

const embedCacheHeaderLen = 8 + 4 + 4 + 4 + 8

func saveEmbeddingCache(path, model string, dim int, hash uint64, vectors [][]float32) error {
	buf := make([]byte, 0, len(vectors)*dim*4+embedCacheHeaderLen+len(model))
	buf = append(buf, embedCacheMagic...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(model)))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(dim))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(vectors)))
	buf = binary.LittleEndian.AppendUint64(buf, hash)
	buf = append(buf, model...)
	for _, v := range vectors {
		for _, f := range v {
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(f))
		}
	}
	return os.WriteFile(path, buf, 0o644)
}

func loadEmbeddingCache(path, model string, dim, count int, hash uint64) ([][]float32, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) < embedCacheHeaderLen || string(data[:8]) != embedCacheMagic {
		return nil, false
	}
	modelLen := int(binary.LittleEndian.Uint32(data[8:12]))
	gotDim := int(binary.LittleEndian.Uint32(data[12:16]))
	gotCount := int(binary.LittleEndian.Uint32(data[16:20]))
	gotHash := binary.LittleEndian.Uint64(data[20:28])
	if gotDim != dim || gotCount != count || gotHash != hash {
		return nil, false
	}
	off := embedCacheHeaderLen
	if off+modelLen > len(data) || string(data[off:off+modelLen]) != model {
		return nil, false
	}
	off += modelLen
	if off+count*dim*4 > len(data) {
		return nil, false
	}
	vectors := make([][]float32, count)
	for i := 0; i < count; i++ {
		v := make([]float32, dim)
		for j := 0; j < dim; j++ {
			v[j] = math.Float32frombits(binary.LittleEndian.Uint32(data[off:]))
			off += 4
		}
		vectors[i] = v
	}
	return vectors, true
}
