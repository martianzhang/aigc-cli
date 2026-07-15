package ideas

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
)

const (
	ideasDirName  = ".config/aigc-cli/ideas"
	ideasFileName = "ideas.json"
)

// SearchText searches the local ideas database for the given keywords and returns formatted results.
func SearchText(keywords string, limit int) (string, error) {
	entries, err := LoadIdeas("")
	if err != nil {
		return "", fmt.Errorf("failed to load ideas: %w", err)
	}
	if len(entries) == 0 {
		return "No ideas found in the database. Run `aigc-cli ideas init` to download.", nil
	}

	idx := BuildBM25Index(entries)

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
	entries, err := LoadIdeas("")
	if err != nil {
		return "", fmt.Errorf("failed to load ideas: %w", err)
	}
	if len(entries) == 0 {
		return "No ideas found in the database. Run `aigc-cli ideas init` to download.", nil
	}

	indices := rand.Perm(len(entries))
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	results := make([]SearchResult, limit)
	for i := 0; i < limit; i++ {
		results[i] = SearchResult{Entry: entries[indices[i]]}
	}
	return FormatResultsMarkdown(results, "random", len(entries)), nil
}

// LoadIdeas reads ideas.json from the given path, or the default location if path is empty.
func LoadIdeas(dataPath string) ([]IdeaEntry, error) {
	path := dataPath
	if path == "" {
		var err error
		path, err = defaultIdeasPath()
		if err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("ideas.json not found at %s.\n  Run 'aigc-cli ideas init' to download the prompt dataset,\n  or place ideas.json at ~/.config/aigc-cli/ideas.json", path)
		}
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var entries []IdeaEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("invalid ideas.json: %w", err)
	}
	return entries, nil
}

// defaultIdeasPath returns the default path to ideas.json.
func defaultIdeasPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ideasDirName, ideasFileName), nil
}
