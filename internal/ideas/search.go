package ideas

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
)

// SearchDBResults searches the database for entries matching keywords.
// Returns ranked SearchResults and the total count of matches.
// The caller is responsible for closing the database.
func SearchDBResults(db *sql.DB, keywords string, limit int) ([]SearchResult, int, error) {
	queryTerms := tokenize(keywords)
	if len(queryTerms) == 0 {
		return nil, 0, nil
	}

	totalDocs, avgDocLen, err := GetCorpusStats(db)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read corpus stats: %w", err)
	}
	if totalDocs == 0 {
		return nil, 0, nil
	}

	globalIDF := make(map[string]float64, len(queryTerms))
	for _, t := range queryTerms {
		df, _, err := GetTermDocFreq(db, t)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to query term %q: %w", t, err)
		}
		fd := float64(df)
		fn := float64(totalDocs)
		globalIDF[t] = math.Log(1 + (fn-fd+0.5)/(fd+0.5))
	}

	candidateIDs, err := QueryCandidateIDs(db, queryTerms)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query candidates: %w", err)
	}
	if len(candidateIDs) == 0 {
		return nil, 0, nil
	}

	entries, err := LoadEntries(db, candidateIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to load candidates: %w", err)
	}
	if len(entries) == 0 {
		return nil, 0, nil
	}

	idx := BuildBM25IndexWithStats(entries, globalIDF, avgDocLen)
	results := searchIdeas(entries, idx, keywords)
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}
	return results, len(results), nil
}

// SearchText searches the local ideas database for the given keywords
// and returns formatted results. Uses SQLite with inverted index for
// candidate filtering, then the same BM25 + n-gram + RRF ranking.
func SearchText(keywords string, limit int) (string, error) {
	db, err := OpenDB("")
	if err != nil {
		return "", fmt.Errorf("failed to open ideas database: %w\n  Run 'aigc-cli ideas init' to download and build it", err)
	}
	defer db.Close()

	queryTerms := tokenize(keywords)
	if len(queryTerms) == 0 {
		return "", fmt.Errorf("no searchable terms in query")
	}

	totalDocs, avgDocLen, err := GetCorpusStats(db)
	if err != nil {
		return "", fmt.Errorf("failed to read corpus stats: %w", err)
	}
	if totalDocs == 0 {
		return "No ideas found in the database. Run `aigc-cli ideas init` to download.", nil
	}

	// Compute IDF for each query term from the full corpus.
	globalIDF := make(map[string]float64, len(queryTerms))
	for _, t := range queryTerms {
		df, _, err := GetTermDocFreq(db, t)
		if err != nil {
			return "", fmt.Errorf("failed to query term %q: %w", t, err)
		}
		fd := float64(df)
		fn := float64(totalDocs)
		globalIDF[t] = math.Log(1 + (fn-fd+0.5)/(fd+0.5))
	}

	// Query inverted index for candidate entry IDs (AND filter).
	candidateIDs, err := QueryCandidateIDs(db, queryTerms)
	if err != nil {
		return "", fmt.Errorf("failed to query candidates: %w", err)
	}
	if len(candidateIDs) == 0 {
		return "No matching prompts found.", nil
	}

	entries, err := LoadEntries(db, candidateIDs)
	if err != nil {
		return "", fmt.Errorf("failed to load candidate entries: %w", err)
	}
	if len(entries) == 0 {
		return "No matching prompts found.", nil
	}

	// Build BM25 index on the candidate set with global IDF.
	idx := BuildBM25IndexWithStats(entries, globalIDF, avgDocLen)

	// Run the full search pipeline (BM25 + n-gram + RRF).
	results := searchIdeas(entries, idx, keywords)
	if len(results) == 0 {
		return "No matching prompts found.", nil
	}

	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	results = results[:limit]

	return FormatResultsMarkdown(results, keywords, len(results)), nil
}

// SearchRandom returns random ideas from the database.
func SearchRandom(limit int) (string, error) {
	db, err := OpenDB("")
	if err != nil {
		return "", fmt.Errorf("failed to open ideas database: %w\n  Run 'aigc-cli ideas init' to download and build it", err)
	}
	defer db.Close()

	entries, err := LoadRandomEntries(db, limit)
	if err != nil {
		return "", fmt.Errorf("failed to load random entries: %w", err)
	}
	if len(entries) == 0 {
		return "No ideas found in the database. Run `aigc-cli ideas init` to download.", nil
	}

	results := make([]SearchResult, len(entries))
	for i, e := range entries {
		results[i] = SearchResult{Entry: e}
	}
	return FormatResultsMarkdown(results, "random", len(entries)), nil
}

// SearchByImage finds entries whose image_urls contain the given filename.
// Kept for backward compatibility — new callers should use SearchEntriesByImage.
func SearchByImage(entries []IdeaEntry, filename string) []SearchResult {
	fn := strings.ToLower(filename)
	seen := make(map[string]bool)
	var results []SearchResult
	for _, e := range entries {
		for _, url := range e.ImageURLs {
			if strings.Contains(strings.ToLower(url), fn) {
				key := url
				if key == "" {
					key = e.SourceURL
				}
				if key == "" {
					key = e.Title + "|" + e.Prompt
				}
				if !seen[key] {
					seen[key] = true
					results = append(results, SearchResult{Entry: e, Score: 1})
				}
				break
			}
		}
	}
	return results
}
