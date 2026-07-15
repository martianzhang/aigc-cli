package ideas

import (
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// --- BM25 + n-gram hybrid search ---

const (
	bm25K1 = 1.2  // BM25 term frequency saturation
	bm25B  = 0.75 // BM25 length normalization
	rrfK   = 30.0 // RRF fusion constant
)

// BM25Index holds precomputed corpus statistics and pre-tokenized data.
type BM25Index = bm25Index

type bm25Index struct {
	avgDocLen float64
	docCount  int
	idf       map[string]float64
	entries   []IdeaEntry

	// Pre-tokenized/cached to avoid re-scanning on every query
	docTokens [][]string       // tokens per doc, for BM25 scoring
	docSet    []map[string]int // token->count per doc, for AND filtering + TF
	docTexts  []string         // pre-computed searchableText, for n-gram
}

// BuildBM25Index walks all entries, tokenizes once (in parallel), and pre-computes everything.
func BuildBM25Index(entries []IdeaEntry) *bm25Index {
	return buildBM25IndexWithStats(entries, nil, 0)
}

// BuildBM25IndexWithStats is like BuildBM25Index but uses pre-computed IDF and
// avgDocLen from the full corpus. This is used when the index is built on a
// subset of entries (search candidates) but needs corpus-level statistics
// for correct BM25 scoring.
// If globalIDF or avgDocLen are zero, falls back to computing them from the
// provided entries (same as BuildBM25Index).
func BuildBM25IndexWithStats(entries []IdeaEntry, globalIDF map[string]float64, avgDocLen float64) *bm25Index {
	return buildBM25IndexWithStats(entries, globalIDF, avgDocLen)
}

func buildBM25IndexWithStats(entries []IdeaEntry, globalIDF map[string]float64, avgDocLen float64) *bm25Index {
	n := len(entries)
	idx := &bm25Index{
		docCount:  n,
		idf:       make(map[string]float64),
		entries:   entries,
		docTokens: make([][]string, n),
		docSet:    make([]map[string]int, n),
		docTexts:  make([]string, n),
	}

	// Pre-compute searchable text (lightweight, serial)
	for i, e := range entries {
		idx.docTexts[i] = searchableText(e)
	}

	// Parallel tokenization
	numWorkers := runtime.NumCPU()
	type workerResult struct {
		totalTokens int
		docFreq     map[string]int
	}

	work := make(chan int, n)
	var wg sync.WaitGroup
	results := make([]workerResult, numWorkers)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		wr := &results[w]
		wr.docFreq = make(map[string]int)

		go func() {
			defer wg.Done()
			for i := range work {
				terms := tokenize(idx.docTexts[i])
				idx.docTokens[i] = terms
				wr.totalTokens += len(terms)

				tf := make(map[string]int, len(terms)/2)
				seen := make(map[string]bool, len(terms)/2)
				for _, t := range terms {
					tf[t]++
					if !seen[t] {
						wr.docFreq[t]++
						seen[t] = true
					}
				}
				idx.docSet[i] = tf
			}
		}()
	}

	for i := 0; i < n; i++ {
		work <- i
	}
	close(work)
	wg.Wait()

	// Merge per-worker results
	totalTokens := 0
	docFreq := make(map[string]int)
	for w := 0; w < numWorkers; w++ {
		wr := results[w]
		totalTokens += wr.totalTokens
		for term, df := range wr.docFreq {
			docFreq[term] += df
		}
	}
	idx.avgDocLen = float64(totalTokens) / float64(max(n, 1))

	if globalIDF != nil {
		// Use pre-computed IDF from the full corpus.
		idx.idf = globalIDF
		idx.avgDocLen = avgDocLen
	} else {
		// Compute IDF from the provided entries.
		for term, df := range docFreq {
			fd := float64(df)
			fn := float64(n)
			idx.idf[term] = math.Log(1 + (fn-fd+0.5)/(fd+0.5))
		}
	}

	return idx
}

// bm25Score returns the BM25 score for a single entry given query terms.
func (idx *bm25Index) bm25Score(entryIdx int, queryTerms []string) float64 {
	tf := idx.docSet[entryIdx]
	docLen := float64(len(idx.docTokens[entryIdx]))
	var score float64

	for _, qt := range queryTerms {
		idf := idx.idf[qt]
		if idf == 0 {
			continue
		}
		freq := float64(tf[qt])
		score += idf * freq * (bm25K1 + 1) / (freq + bm25K1*(1-bm25B+bm25B*docLen/idx.avgDocLen))
	}

	return score
}

// ngramSet returns the set of character n-grams for a string.
func ngramSet(s string, n int) map[string]int {
	s = strings.ToLower(s)
	grams := make(map[string]int)
	for i := 0; i <= len(s)-n; i++ {
		grams[s[i:i+n]]++
	}
	return grams
}

// cosineSimilarity computes cosine similarity between two n-gram frequency maps.
func cosineSimilarity(a, b map[string]int) float64 {
	var dot, normA, normB float64
	for k, v := range a {
		normA += float64(v * v)
		if bv, ok := b[k]; ok {
			dot += float64(v * bv)
		}
	}
	for _, v := range b {
		normB += float64(v * v)
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// searchableText combines all searchable fields into one string.
func searchableText(e IdeaEntry) string {
	return e.Title + " " + e.TitleZh + " " + e.Prompt + " " + e.PromptZh
}

// tokenize splits text into lowercase tokens.
// ASCII sequences get fast-path single tokens (min 2 chars).
// CJK sequences are split into overlapping 2-grams.
func tokenize(text string) []string {
	var tokens []string
	var buf []rune
	for _, r := range text {
		// ASCII letter/digit -> fast path, no unicode table lookup
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			buf = append(buf, r)
		} else if r >= 'A' && r <= 'Z' {
			buf = append(buf, r+32) // inline lowercase
		} else if r < 0x80 {
			if len(buf) > 0 {
				tokens = append(tokens, splitTokens(buf)...)
				buf = buf[:0]
			}
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf = append(buf, unicode.ToLower(r))
		} else if len(buf) > 0 {
			tokens = append(tokens, splitTokens(buf)...)
			buf = buf[:0]
		}
	}
	if len(buf) > 0 {
		tokens = append(tokens, splitTokens(buf)...)
	}
	return tokens
}

// splitTokens converts a rune buffer into one or more tokens.
// CJK-only buffers longer than 2 are split into overlapping 2-grams.
func splitTokens(buf []rune) []string {
	if len(buf) == 0 {
		return nil
	}
	if len(buf) == 1 {
		return []string{string(buf)}
	}
	allCJK := true
	for _, r := range buf {
		if !unicode.Is(unicode.Han, r) && !unicode.Is(unicode.Hiragana, r) &&
			!unicode.Is(unicode.Katakana, r) && !unicode.Is(unicode.Hangul, r) {
			allCJK = false
			break
		}
	}
	if allCJK && len(buf) > 2 {
		grams := make([]string, 0, len(buf)-1)
		for i := 0; i < len(buf)-1; i++ {
			grams = append(grams, string(buf[i:i+2]))
		}
		return grams
	}
	return []string{string(buf)}
}

// containsWord checks if term appears as a whole word in text.
// Uses byte-level word boundary detection that works for both
// ASCII (space-separated) and CJK (no spaces between characters).
func containsWord(text, term string) bool {
	lower := strings.ToLower(text)
	t := strings.ToLower(strings.TrimSpace(term))
	if t == "" {
		return false
	}
	// Scan for term at every position with word-boundary check
	for i := 0; i <= len(lower)-len(t); i++ {
		if lower[i:i+len(t)] != t {
			continue
		}
		if i > 0 && isWordChar(lower[i-1]) {
			continue
		}
		if i+len(t) < len(lower) && isWordChar(lower[i+len(t)]) {
			continue
		}
		return true
	}
	return false
}

// isWordChar returns true for ASCII word characters (a-z, A-Z, 0-9).
// Non-ASCII bytes (CJK, punctuation) are NOT word chars, which means
// CJK characters are naturally treated as word boundaries.
func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// andFilter returns true if all query terms exist in the pre-tokenized doc set.
func andFilter(docSet map[string]int, terms []string) bool {
	for _, t := range terms {
		if _, ok := docSet[t]; !ok {
			return false
		}
	}
	return true
}

// --- search ---

func searchIdeas(entries []IdeaEntry, idx *bm25Index, query string) []SearchResult {
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	// Phase 1: AND filter (via pre-tokenized set) + BM25 scoring
	type scored struct {
		entryIdx int
		bm25     float64
	}
	var candidates []scored

	for i := range entries {
		if !andFilter(idx.docSet[i], queryTerms) {
			continue
		}
		s := idx.bm25Score(i, queryTerms)
		titleBoost := false
		for _, t := range queryTerms {
			if containsWord(entries[i].Title+" "+entries[i].TitleZh, t) {
				titleBoost = true
				break
			}
		}
		if titleBoost {
			s *= 2
		}

		candidates = append(candidates, scored{entryIdx: i, bm25: s})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Phase 2: n-gram cosine similarity
	queryNgrams := ngramSet(query, 3)
	type ngramScored struct {
		entryIdx int
		cosine   float64
	}
	var ngramCandidates []ngramScored
	for _, c := range candidates {
		docNgrams := ngramSet(idx.docTexts[c.entryIdx], 3)
		cos := cosineSimilarity(queryNgrams, docNgrams)
		ngramCandidates = append(ngramCandidates, ngramScored{entryIdx: c.entryIdx, cosine: cos})
	}

	// Phase 3: RRF fusion
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].bm25 > candidates[j].bm25
	})
	bm25Ranks := make(map[int]int)
	for i, c := range candidates {
		bm25Ranks[c.entryIdx] = i + 1
	}

	sort.Slice(ngramCandidates, func(i, j int) bool {
		return ngramCandidates[i].cosine > ngramCandidates[j].cosine
	})
	ngramRanks := make(map[int]int)
	for i, c := range ngramCandidates {
		ngramRanks[c.entryIdx] = i + 1
	}

	rrf := make(map[int]float64)
	for _, c := range candidates {
		r1 := float64(bm25Ranks[c.entryIdx])
		r2 := float64(ngramRanks[c.entryIdx])
		rrf[c.entryIdx] = 1.0/(rrfK+r1) + 1.0/(rrfK+r2)
	}

	results := make([]SearchResult, 0, len(candidates))
	for _, c := range candidates {
		results = append(results, SearchResult{
			Entry: entries[c.entryIdx],
			Score: int(rrf[c.entryIdx] * 1000),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

// SearchIdeas searches the indexed entries by keywords.
func SearchIdeas(entries []IdeaEntry, idx *bm25Index, query string) []SearchResult {
	return searchIdeas(entries, idx, query)
}
