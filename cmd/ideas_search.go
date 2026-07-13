package cmd

import (
	"fmt"
	"math/rand"
)

// searchIdeasText searches the local ideas database and returns formatted results.
// Shared by CLI and agent loop.
func searchIdeasText(keywords string, limit int) (string, error) {
	entries, rawData, err := loadIdeas()
	if err != nil {
		return "", fmt.Errorf("failed to load ideas: %w", err)
	}

	if len(entries) == 0 {
		return "No ideas found in the database. Run `aigc-cli ideas init` to download.", nil
	}

	// Load or build BM25 index
	hash := computeHash(rawData)
	idx := loadCachedIndex(shared.Cfg, hash)
	if idx == nil {
		idx = buildBM25Index(entries)
		saveCachedIndex(shared.Cfg, idx, hash)
	}

	results := searchIdeas(entries, idx, keywords)
	if len(results) == 0 {
		return "No matching prompts found.", nil
	}

	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	results = results[:limit]

	return formatResultsMarkdown(results, keywords, len(results)), nil
}

// searchIdeasRandom returns random ideas from the database.
func searchIdeasRandom(limit int) (string, error) {
	entries, _, err := loadIdeas()
	if err != nil {
		return "", fmt.Errorf("failed to load ideas: %w", err)
	}
	if len(entries) == 0 {
		return "No ideas found in the database. Run `aigc-cli ideas init` to download.", nil
	}

	// Shuffle and pick
	indices := rand.Perm(len(entries))
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	results := make([]searchResult, limit)
	for i := 0; i < limit; i++ {
		results[i] = searchResult{entry: entries[indices[i]]}
	}
	return formatResultsMarkdown(results, "random", len(entries)), nil
}
