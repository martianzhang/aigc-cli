// Package ideas provides BM25-powered search over a local prompt ideas dataset.
package ideas

// IdeaEntry represents a single prompt entry in ideas.json.
type IdeaEntry struct {
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

// SearchResult pairs an entry with its relevance score.
type SearchResult struct {
	Entry IdeaEntry
	Score int
}
