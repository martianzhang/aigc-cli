package knowledge

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// HashEmbedder creates embeddings via random projection of character n-grams.
// No external model needed — pure Go, deterministic, works for any language.
//
// The technique is inspired by "Hash Embed" / random indexing:
//
//	Each n-gram (char or word) is hashed to a random 384-d vector.
//	A document's embedding is the weighted sum of its n-gram vectors,
//	normalized to unit length.
//
// For CJK text, character bigrams and trigrams are used.
// For alphabetic text, word unigrams and bigrams are used.
type HashEmbedder struct {
	dim  int
	seed uint64
}

// NewHashEmbedder creates a hash embedder with the given dimension.
func NewHashEmbedder(dim int) *HashEmbedder {
	if dim <= 0 {
		dim = 384
	}
	// Use a fixed seed so embeddings are deterministic across runs.
	// 42 is not special — any fixed value works.
	return &HashEmbedder{dim: dim, seed: 42}
}

// Dim returns the embedding dimension.
func (h *HashEmbedder) Dim() int { return h.dim }

// Embed generates a 384-d embedding for the given text.
func (h *HashEmbedder) Embed(text string) (Embedding, error) {
	ngrams := h.extractNGrams(text)
	vec := make([]float64, h.dim)

	const scale = 0.1
	for _, ng := range ngrams {
		v := h.hashToVector(ng)
		for i := 0; i < h.dim; i++ {
			vec[i] += float64(v[i]) * scale
		}
	}

	// Normalize to unit length
	norm := 0.0
	for _, v := range vec {
		norm += v * v
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for i := range vec {
			vec[i] /= norm
		}
	}

	var emb Embedding
	for i := 0; i < h.dim && i < len(vec); i++ {
		emb[i] = float32(vec[i])
	}
	return emb, nil
}

// extractNGrams extracts character and word n-grams from text.
func (h *HashEmbedder) extractNGrams(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil
	}

	var ngrams []string
	seen := make(map[string]bool)

	// Detect if text is primarily CJK
	cjkCount := 0
	totalRunes := 0
	for _, r := range text {
		totalRunes++
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || (r >= 0xAC00 && r <= 0xD7AF) {
			cjkCount++
		}
	}
	isCJK := cjkCount > totalRunes/2

	if isCJK {
		// Character bigrams and trigrams for CJK
		runes := []rune(text)
		for i := 0; i < len(runes); i++ {
			if unicode.IsSpace(runes[i]) || unicode.IsPunct(runes[i]) {
				continue
			}
			// Unigram
			ng := string(runes[i])
			if !seen[ng] {
				ngrams = append(ngrams, ng)
				seen[ng] = true
			}
			// Bigram
			if i+1 < len(runes) {
				ng = string(runes[i]) + string(runes[i+1])
				if !seen[ng] {
					ngrams = append(ngrams, ng)
					seen[ng] = true
				}
			}
			// Trigram
			if i+2 < len(runes) {
				ng = string(runes[i]) + string(runes[i+1]) + string(runes[i+2])
				if !seen[ng] {
					ngrams = append(ngrams, ng)
					seen[ng] = true
				}
			}
		}
	} else {
		// Word unigrams and bigrams for alphabetic text
		words := tokenizeWords(text)
		for i, w := range words {
			if len(w) <= 2 {
				continue // skip very short words
			}
			if !seen[w] {
				ngrams = append(ngrams, w)
				seen[w] = true
			}
			// Word bigram
			if i+1 < len(words) {
				bg := w + " " + words[i+1]
				if !seen[bg] {
					ngrams = append(ngrams, bg)
					seen[bg] = true
				}
			}
		}
	}

	return ngrams
}

// hashToVector generates a random unit-binary vector for a given n-gram.
// Same n-gram always produces the same vector (deterministic).
func (h *HashEmbedder) hashToVector(ngram string) []float32 {
	fnvHash := fnv.New64a()
	fnvHash.Write([]byte(ngram)) //nolint:errcheck
	h1 := fnvHash.Sum64()

	// Use splitmix64 to expand to dimension values
	vec := make([]float32, h.dim)
	state := h1 ^ h.seed
	for i := 0; i < h.dim; i++ {
		state += 0x9e3779b97f4a7c15
		z := state
		z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
		z = (z ^ (z >> 27)) * 0x94d049bb133111eb
		z = z ^ (z >> 31)
		// Map to {-1, 0, +1} randomly
		vec[i] = float32(int64(z)%3 - 1)
	}
	return vec
}

// tokenizeWords splits text into words, handling punctuation.
func tokenizeWords(text string) []string {
	var words []string
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			words = append(words, buf.String())
			buf.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '\'' || r == '-' {
			buf.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return words
}

// NormalizeEmbedding normalizes an embedding vector to unit length.
func NormalizeEmbedding(e *Embedding) {
	var norm float64
	for _, v := range e {
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		return
	}
	norm = math.Sqrt(norm)
	for i := range e {
		e[i] = float32(float64(e[i]) / norm)
	}
}

// MeanEmbedding computes the mean of multiple embeddings.
func MeanEmbedding(embeddings []Embedding) Embedding {
	if len(embeddings) == 0 {
		return Embedding{}
	}
	var sum Embedding
	for _, e := range embeddings {
		for i := range sum {
			sum[i] += e[i]
		}
	}
	n := float32(len(embeddings))
	for i := range sum {
		sum[i] /= n
	}
	return sum
}

// BestEmbedder tries to create an ONNX embedder, falling back to HashEmbedder.
// modelsDir is the shared models directory (e.g. ~/.config/aigc-cli/models).
// onnxLibPath is the ONNX Runtime shared library path (can be empty).
func BestEmbedder(modelsDir, onnxLibPath string) Embedder {
	if onnxLibPath != "" {
		modelDir := EmbedModelDir(modelsDir)
		e, err := NewONNXEmbedder(modelDir, onnxLibPath)
		if err == nil {
			return e
		}
	}
	return NewHashEmbedder(384)
}

// ensure utf8 is used
var _ = utf8.ValidString
