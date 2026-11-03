// Package ideas provides BM25-powered search over a local prompt ideas dataset.
package ideas

import (
	"crypto/sha256"
	"encoding/hex"
)

// idLength is the number of hex characters in a generated idea ID. 12 hex
// characters (48 bits) keep collisions negligible for the ~30K-entry dataset
// while staying short enough to copy by hand.
const idLength = 12

// IdeaEntry represents a single prompt entry in ideas.json.
type IdeaEntry struct {
	ID        string   `json:"id,omitempty"`
	Title     string   `json:"title,omitempty"`
	TitleZh   string   `json:"title_zh,omitempty"`
	Prompt    string   `json:"prompt"`
	PromptZh  string   `json:"prompt_zh,omitempty"`
	ImageURLs []string `json:"image_urls,omitempty"`
	SourceURL string   `json:"source_url,omitempty"`
	Author    string   `json:"author,omitempty"`
	License   string   `json:"license,omitempty"`
	Lang      string   `json:"lang"`
}

// StableID returns the entry's stable identifier. An explicit `id` field wins;
// otherwise the ID is a short hash of source_url + prompt, so it survives
// dataset reordering and updates as long as the entry content is unchanged.
func (e IdeaEntry) StableID() string {
	if e.ID != "" {
		return e.ID
	}
	sum := sha256.Sum256([]byte(e.SourceURL + "\x00" + e.Prompt))
	return hex.EncodeToString(sum[:])[:idLength]
}

// DisplayPrompt returns the prompt to show the user: prompt_zh for Chinese
// entries that have a translation, otherwise the original prompt.
func (e IdeaEntry) DisplayPrompt() string {
	if e.Lang == "zh" && e.PromptZh != "" {
		return e.PromptZh
	}
	return e.Prompt
}

// SearchResult pairs an entry with its relevance score.
type SearchResult struct {
	Entry IdeaEntry
	Score int
}
