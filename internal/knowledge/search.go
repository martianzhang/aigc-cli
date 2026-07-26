package knowledge

import (
	"fmt"
	"math"
	"strings"
)

// ftsReserved are characters FTS5 treats as operators.
// These must be removed from unquoted terms to avoid syntax errors.
const ftsReserved = `+-"*()~`

// SearchQuery represents a parsed search query.
type SearchQuery struct {
	Raw       string
	Processed string // FTS5-safe query
}

// ParseSearchQuery parses a raw query string for FTS5 search.
// Handles: hyphenated terms (aigc-cli), special chars, multi-word phrases.
func ParseSearchQuery(raw string) SearchQuery {
	// Split into words, preserving quoted phrases
	rawTerms := tokenizeFTS(raw)
	var processedTerms []string

	for _, t := range rawTerms {
		if t == "" {
			continue
		}

		// Terms with these characters can confuse FTS5 syntax and need quoting.
		if needsQuoting(t) {
			cleaned := stripFTSReserved(t, true)
			if cleaned != "" {
				processedTerms = append(processedTerms, `"`+cleaned+`"`)
			}
			continue
		}

		// Short terms: prefix match with *
		cleaned := stripFTSReserved(t, false)
		if cleaned == "" {
			continue
		}
		if len(cleaned) <= 3 {
			processedTerms = append(processedTerms, cleaned+"*")
		} else {
			processedTerms = append(processedTerms, cleaned)
		}
	}

	processed := strings.Join(processedTerms, " ")
	if processed == "" {
		// Fallback: strip all reserved chars and use as-is
		processed = stripFTSReserved(raw, false)
	}
	if processed == "" {
		processed = raw
	}

	return SearchQuery{Raw: raw, Processed: processed}
}

// tokenizeFTS splits a query into terms, respecting double-quoted phrases.
func tokenizeFTS(q string) []string {
	var terms []string
	var buf strings.Builder
	inQuote := false

	for _, r := range q {
		switch r {
		case '"':
			inQuote = !inQuote
			buf.WriteRune(r)
		case ' ', '\t', '\n':
			if inQuote {
				buf.WriteRune(r)
			} else {
				if buf.Len() > 0 {
					terms = append(terms, buf.String())
					buf.Reset()
				}
			}
		default:
			buf.WriteRune(r)
		}
	}
	if buf.Len() > 0 {
		terms = append(terms, buf.String())
	}
	return terms
}

// stripFTSReserved removes FTS5 operator characters from a term.
// When keepHyphen is true, hyphens are preserved (for quoted terms).
// ftsQuoteChars are characters that when present in a term require
// the whole term to be double-quoted for FTS5 to parse it safely.
const ftsQuoteChars = "-."

func needsQuoting(s string) bool {
	return strings.ContainsAny(s, ftsQuoteChars)
}

func stripFTSReserved(s string, keepHyphen bool) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(ftsReserved, r) {
			if keepHyphen && r == '-' {
				b.WriteRune(r)
				continue
			}
			if r == '"' {
				// Keep quotes for readability, they'll be in the output
				continue
			}
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// Search performs FTS5-first search, supplemented by vector when needed.
// Search runs FTS5 and vector search in parallel, fusing results.
// FTS5 exact matches are always included. Vector-only results filtered by min_score.
func (s *Store) Search(query string, limit int, project string) (SearchResults, error) {
	if limit <= 0 {
		limit = 10
	}

	sq := ParseSearchQuery(query)
	ftsResults, ftsErr := s.SearchFTS(sq.Processed, limit*3, project)

	if ftsErr == nil && len(ftsResults) == 0 {
		simplified := simplifyFTSQuery(sq.Processed)
		if simplified != "" && simplified != sq.Processed {
			ftsResults, ftsErr = s.SearchFTS(simplified, limit*3, project)
		}
	}

	if ftsErr == nil && len(ftsResults) == 0 {
		orQuery := orFTSQuery(sq.Processed)
		if orQuery != "" && orQuery != sq.Processed {
			ftsResults, ftsErr = s.SearchFTS(orQuery, limit*3, project)
		}
	}

	if ftsErr != nil {
		return nil, fmt.Errorf("FTS search: %w", ftsErr)
	}

	embedder := s.embedder
	if embedder == nil {
		embedder = NewHashEmbedder(384)
	}
	qVec, _ := embedder.Embed(query)
	vectorResults, vecErr := s.SearchVector(qVec, limit*3, project)

	type scored struct {
		result SearchResult
		score  float64
	}
	chunkMap := make(map[int64]*scored)

	// FTS5 results get rank-based scores
	for i, r := range ftsResults {
		s := float64(limit-i) / float64(limit)
		chunkMap[r.Chunk.ID] = &scored{result: r, score: s}
	}

	// Merge vector results
	if vecErr == nil {
		for _, vr := range vectorResults {
			if existing, ok := chunkMap[vr.Chunk.ID]; ok {
				// In both lists: use max score + keyword boost
				if vr.Score > existing.score {
					existing.score = vr.Score + 0.2
				} else {
					existing.score = existing.score + 0.2
				}
			} else {
				if s.minScore > 0 && vr.Score < s.minScore {
					continue
				}
				vr.Score = vr.Score * 0.5
				chunkMap[vr.Chunk.ID] = &scored{result: vr, score: vr.Score}
			}
		}
	}

	results := make([]*scored, 0, len(chunkMap))
	for _, sc := range chunkMap {
		results = append(results, sc)
	}
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if limit > len(results) {
		limit = len(results)
	}
	results = results[:limit]

	final := make(SearchResults, len(results))
	for i, sc := range results {
		sc.result.Score = sc.score
		final[i] = sc.result
	}
	return final, nil
}

// simplifyFTSQuery removes very short terms (1-2 chars) from an FTS query
// to improve recall when the original query returned no results.
func simplifyFTSQuery(query string) string {
	terms := strings.Fields(query)
	var kept []string
	for _, t := range terms {
		cleaned := strings.TrimRight(t, "*")
		if len(cleaned) <= 2 {
			continue
		}
		kept = append(kept, t)
	}
	return strings.Join(kept, " ")
}

// orFTSQuery converts an AND query to OR for when terms span different chunks.
func orFTSQuery(query string) string {
	terms := strings.Fields(query)
	if len(terms) <= 1 {
		return ""
	}
	var kept []string
	for _, t := range terms {
		cleaned := strings.TrimRight(t, "*")
		if len(cleaned) >= 3 {
			kept = append(kept, t)
		}
	}
	if len(kept) <= 1 {
		return ""
	}
	return strings.Join(kept, " OR ")
}

// rrfFuse merges two ranked lists using Reciprocal Rank Fusion.
func rrfFuse(a, b SearchResults, limit int) SearchResults {
	const k = 60 // RRF constant

	// Collect unique chunk IDs with RRF scores
	type scoredChunk struct {
		result SearchResult
		rrf    float64
	}
	chunkMap := make(map[int64]*scoredChunk)
	seen := make(map[int64]bool)

	addWithRank := func(results SearchResults, offset int) {
		for i, r := range results {
			if seen[r.Chunk.ID] {
				if cs, ok := chunkMap[r.Chunk.ID]; ok {
					cs.rrf += 1.0 / (float64(i+1+offset) + k)
				}
				continue
			}
			seen[r.Chunk.ID] = true
			chunkMap[r.Chunk.ID] = &scoredChunk{
				result: r,
				rrf:    1.0 / (float64(i+1+offset) + k),
			}
		}
	}

	addWithRank(a, 0)
	addWithRank(b, len(a)+1)

	// Collect and sort
	results := make(SearchResults, 0, len(chunkMap))
	for _, cs := range chunkMap {
		cs.result.Score = cs.rrf
		results = append(results, cs.result)
	}

	sortResults(results)
	if limit < len(results) {
		results = results[:limit]
	}
	return results
}

// rerankByFTS returns FTS results sorted by score descending.

// ---------- ONNX Reranker ----------

// RerankerScorer scores a query-document pair for re-ranking.
type RerankerScorer func(query, document string) float64

// availableReranker is nil when not built with CGO support.
var availableReranker RerankerScorer

// SetReranker sets the global reranker function.
// Called from the CGO-enabled init path.
func SetReranker(fn RerankerScorer) {
	availableReranker = fn
}

// Rerank applies the reranker to re-score and re-sort results.
// If no reranker is available, returns results unchanged.
func Rerank(query string, results SearchResults, topK int) SearchResults {
	if availableReranker == nil {
		return results
	}
	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}

	for i := range results {
		text := results[i].Chunk.Heading + " " + results[i].Chunk.Content
		score := availableReranker(query, text)
		if !math.IsNaN(score) && !math.IsInf(score, 0) {
			results[i].Score = score
		}
	}

	sortResults(results)
	if topK < len(results) {
		results = results[:topK]
	}
	return results
}
func sortResults(results SearchResults) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
