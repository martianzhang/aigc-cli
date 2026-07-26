package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/spf13/cobra"
)

var kbSearchProviderFlag string

var kbSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Find + web search (search local KB then fetch latest from web)",
	Long: `Combines local knowledge base search with web search.

  1. Search local KB for matching documents (same as kb find)
  2. Search the web for latest content using configured search router
  3. Save new web results to KB
  4. Output all results

Configure web_search providers in config.yaml for automatic fallback.
Supports: duckduckgo (built-in), firecrawl, brave (need API key).`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		verbose := shared.Verbose

		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer store.Close()

		project := resolveSearchProject(cmd)

		// Step 1: Search local KB
		localResults, err := store.Search(query, 10, project)
		if err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "Local search error: %v\n", err)
			}
		} else if len(localResults) > 0 {
			outputLocalResults(localResults, verbose)
		} else if verbose {
			fmt.Fprintf(os.Stderr, "No local results found.\n")
		}

		// Step 2: Search web using router
		if err := searchWebWithRouter(store, cmd, query, project, verbose); err != nil {
			return err
		}

		return nil
	},
}

// searchWebWithRouter uses the search router or falls back to direct providers.
func searchWebWithRouter(store *knowledge.Store, cmd *cobra.Command, query, project string, verbose bool) error {
	// Check if --provider was explicitly set
	if cmd.Flags().Changed("provider") {
		provider, _ := cmd.Flags().GetString("provider")
		return searchWithProvider(store, cmd, query, project, provider, verbose)
	}

	// Check if web_search config exists
	if shared.Cfg != nil && len(shared.Cfg.WebSearch) > 0 {
		return searchViaRouter(store, cmd, query, project, verbose)
	}

	// Fallback: use default provider from config or duckduckgo
	provider := resolveSearchProvider(cmd)
	return searchWithProvider(store, cmd, query, project, provider, verbose)
}

// searchViaRouter uses the search router with configured providers.
func searchViaRouter(store *knowledge.Store, cmd *cobra.Command, query, project string, verbose bool) error {
	// Create quota store from KB database
	qStore, err := search.NewQuotaStore(store.DB())
	if err != nil {
		return fmt.Errorf("quota store: %w", err)
	}

	router := search.NewRouter(qStore)

	// Register configured providers
	for name, cfg := range shared.Cfg.WebSearch {
		info := search.ConfigFromTypes(map[string]*types.WebSearchProvider{name: cfg})[name]
		switch cfg.Type {
		case "duckduckgo":
			router.Register(name, search.NewDDGProvider(), info)
		case "firecrawl":
			if cfg.APIKey != "" {
				router.Register(name, search.NewFirecrawlProvider(cfg.APIKey), info)
			} else if apiKey := os.Getenv("FIRECRAWL_API_KEY"); apiKey != "" {
				router.Register(name, search.NewFirecrawlProvider(apiKey), info)
			}
		case "brave":
			if cfg.APIKey != "" {
				router.Register(name, search.NewBraveProvider(cfg.APIKey), info)
			}
		}
	}

	results, err := router.Search(query, 3, "auto", nil)
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "Web search error: %v\n", err)
		}
		return nil
	}

	return fetchSearchResults(store, cmd, results, query, project, verbose)
}

// searchWithProvider uses a single named provider.
func searchWithProvider(store *knowledge.Store, cmd *cobra.Command, query, project, provider string, verbose bool) error {
	switch strings.ToLower(provider) {
	case "firecrawl":
		return searchFirecrawl(store, query, project, verbose)
	default:
		return searchDuckDuckGo(store, cmd, query, project, verbose)
	}
}

func fetchSearchResults(store *knowledge.Store, cmd *cobra.Command, results []search.Result, query, project string, verbose bool) error {
	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder := knowledge.NewHashEmbedder(384)

	for _, sr := range results {
		if verbose {
			fmt.Fprintf(os.Stderr, "  Fetching: %s\n", sr.URL)
		}
		time.Sleep(500 * time.Millisecond)

		fetchResult, fetchErr := knowledge.FetchURL(sr.URL)
		if fetchErr != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Skip: %v\n", fetchErr)
			}
			continue
		}

		docID := knowledge.Checksum(fetchResult.Content)
		existing, _ := store.GetDocument(docID)
		if existing != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Already in KB\n")
			}
			if content := tryReadDoc(docID); content != "" {
				outputDoc(docID, fetchResult.Title, fetchResult.URL, content)
			} else {
				outputDoc(docID, fetchResult.Title, fetchResult.URL, fetchResult.Content)
			}
			continue
		}

		doc := &knowledge.Document{
			ID: docID, URL: fetchResult.URL, Title: fetchResult.Title,
			Project: project, Size: fetchResult.Size, Checksum: docID,
		}
		if err := store.SaveDocument(doc); err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Error saving: %v\n", err)
			}
			continue
		}
		knowledge.SaveDocFile(kbBaseDir, project, docID, fetchResult.Title, fetchResult.Content)

		rawChunks := chunker.Chunk(fetchResult.Content)
		embeddings := make([]knowledge.Embedding, len(rawChunks))
		for i, c := range rawChunks {
			emb, err := embedder.Embed(c.Content)
			if err != nil {
				continue
			}
			embeddings[i] = emb
		}
		if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Error saving chunks: %v\n", err)
			}
			continue
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "    Saved: %s\n", fetchResult.Title)
		}
		outputDoc(docID, fetchResult.Title, fetchResult.URL, fetchResult.Content)
	}
	return nil
}

func resolveSearchProvider(cmd *cobra.Command) string {
	if cmd.Flags().Changed("provider") {
		v, _ := cmd.Flags().GetString("provider")
		if v != "" {
			return v
		}
	}
	if shared.Cfg != nil && shared.Cfg.Defaults != nil && shared.Cfg.Defaults.Knowledgebase != nil {
		if p := shared.Cfg.Defaults.Knowledgebase.SearchProvider; p != "" {
			return p
		}
	}
	return "duckduckgo"
}

func init() {
	kbSearchCmd.Flags().StringVar(&kbSearchProviderFlag, "provider", "", "Search provider: duckduckgo, firecrawl (overrides config defaults)")
	kbCmd.AddCommand(kbSearchCmd)
}

func outputDoc(docID, title, url, content string) {
	fmt.Println("---")
	fmt.Printf("id: %s\n", docID[:12])
	fmt.Printf("title: %s\n", title)
	fmt.Printf("url: %s\n", url)
	fmt.Println(content)
}

func tryReadDoc(docID string) string {
	docsDir := filepath.Join(kbBaseDir, "docs")
	var found string
	filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), docID[:12]) {
			found = path
			return fmt.Errorf("stop")
		}
		return nil
	})
	if found == "" {
		return ""
	}
	data, err := os.ReadFile(found)
	if err != nil {
		return ""
	}
	return string(data)
}

func outputLocalResults(results knowledge.SearchResults, verbose bool) {
	// Aggregate by document
	type docRes struct {
		docID  string
		title  string
		source string
		score  float64
		nchunk int
	}
	docMap := make(map[string]*docRes)
	for _, r := range results {
		d, ok := docMap[r.Document.ID]
		if !ok {
			source := r.Document.URL
			if source == "" {
				source = r.Document.FilePath
			}
			docMap[r.Document.ID] = &docRes{
				docID: r.Document.ID[:12], title: r.Document.Title,
				source: source, score: r.Score,
			}
			d = docMap[r.Document.ID]
		}
		if r.Score > d.score {
			d.score = r.Score
		}
		d.nchunk++
	}

	docs := make([]*docRes, 0, len(docMap))
	for _, d := range docMap {
		docs = append(docs, d)
	}
	for i := 0; i < len(docs); i++ {
		for j := i + 1; j < len(docs); j++ {
			if docs[j].score > docs[i].score {
				docs[i], docs[j] = docs[j], docs[i]
			}
		}
	}

	if len(docs) == 0 {
		return
	}

	fmt.Printf("From knowledge base:\n\n")
	for i, d := range docs {
		fmt.Printf("[%d] %s\n", i+1, d.title)
		fmt.Printf("    id: %s | source: %s | score: %.4f (%d chunk(s))\n", d.docID, d.source, d.score, d.nchunk)
	}
	fmt.Println()
}

func searchDuckDuckGo(store *knowledge.Store, cmd *cobra.Command, query string, project string, verbose bool) error {
	urls, err := knowledge.DDGSearchURLs(query)
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}
	if len(urls) == 0 {
		return nil // no web results is not a fatal error
	}

	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder := knowledge.NewHashEmbedder(384)

	if verbose {
		fmt.Fprintf(os.Stderr, "Found %d result(s) for %q\n", len(urls), query)
	}

	webHeader := func() {
		fmt.Println("\nFrom web search:")
	}

	for _, resultURL := range urls {
		if verbose {
			fmt.Fprintf(os.Stderr, "  Fetching: %s\n", resultURL)
		}
		time.Sleep(300 * time.Millisecond)

		fetchResult, fetchErr := knowledge.FetchURL(resultURL)
		if fetchErr != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Skip: %v\n", fetchErr)
			}
			continue
		}

		docID := knowledge.Checksum(fetchResult.Content)
		existing, _ := store.GetDocument(docID)

		if existing != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    From store\n")
			}
			webHeader()
			if content := tryReadDoc(docID); content != "" {
				outputDoc(docID, fetchResult.Title, fetchResult.URL, content)
			} else {
				outputDoc(docID, fetchResult.Title, fetchResult.URL, fetchResult.Content)
			}
			continue
		}

		webHeader()

		doc := &knowledge.Document{
			ID:       docID,
			URL:      fetchResult.URL,
			Title:    fetchResult.Title,
			Project:  project,
			Size:     fetchResult.Size,
			Checksum: docID,
		}
		if err := store.SaveDocument(doc); err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Error saving: %v\n", err)
			}
			continue
		}
		knowledge.SaveDocFile(kbBaseDir, project, docID, fetchResult.Title, fetchResult.Content)

		rawChunks := chunker.Chunk(fetchResult.Content)
		embeddings := make([]knowledge.Embedding, len(rawChunks))
		for i, c := range rawChunks {
			emb, err := embedder.Embed(c.Content)
			if err != nil {
				continue
			}
			embeddings[i] = emb
		}
		if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Error saving chunks: %v\n", err)
			}
			continue
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "    Saved to KB\n")
		}
		outputDoc(docID, fetchResult.Title, fetchResult.URL, fetchResult.Content)
	}

	return nil
}

func searchFirecrawl(store *knowledge.Store, query string, project string, verbose bool) error {
	apiKey := os.Getenv("FIRECRAWL_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("FIRECRAWL_API_KEY env var required")
	}

	body := map[string]interface{}{
		"query":   query,
		"limit":   3,
		"sources": []map[string]string{{"type": "web"}},
	}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", "https://api.firecrawl.dev/v1/search", strings.NewReader(string(bodyJSON)))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("firecrawl search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firecrawl API error %d: %s", resp.StatusCode, string(respBody))
	}

	var fcResp struct {
		Data struct {
			Results []struct {
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fcResp); err != nil {
		return fmt.Errorf("parse firecrawl response: %w", err)
	}
	if len(fcResp.Data.Results) == 0 {
		return fmt.Errorf("no search results found for %q", query)
	}

	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder := knowledge.NewHashEmbedder(384)

	if verbose {
		fmt.Fprintf(os.Stderr, "Found %d result(s) via Firecrawl\n", len(fcResp.Data.Results))
	}

	webHeader := func() {
		fmt.Println("\nFrom web search:")
	}

	for _, r := range fcResp.Data.Results {
		if verbose {
			fmt.Fprintf(os.Stderr, "  Fetching: %s\n", r.URL)
		}
		time.Sleep(300 * time.Millisecond)

		fetchResult, fetchErr := knowledge.FetchURL(r.URL)
		if fetchErr != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Skip: %v\n", fetchErr)
			}
			continue
		}

		docID := knowledge.Checksum(fetchResult.Content)
		existing, _ := store.GetDocument(docID)

		if existing != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    From store\n")
			}
			webHeader()
			if content := tryReadDoc(docID); content != "" {
				outputDoc(docID, fetchResult.Title, fetchResult.URL, content)
			} else {
				outputDoc(docID, fetchResult.Title, fetchResult.URL, fetchResult.Content)
			}
			continue
		}

		webHeader()

		doc := &knowledge.Document{
			ID:       docID,
			URL:      fetchResult.URL,
			Title:    fetchResult.Title,
			Size:     fetchResult.Size,
			Checksum: docID,
		}
		if err := store.SaveDocument(doc); err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Error saving: %v\n", err)
			}
			continue
		}
		knowledge.SaveDocFile(kbBaseDir, "", docID, fetchResult.Title, fetchResult.Content)

		rawChunks := chunker.Chunk(fetchResult.Content)
		embeddings := make([]knowledge.Embedding, len(rawChunks))
		for i, c := range rawChunks {
			emb, err := embedder.Embed(c.Content)
			if err != nil {
				continue
			}
			embeddings[i] = emb
		}
		if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    Error saving chunks: %v\n", err)
			}
			continue
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "    Saved to KB\n")
		}
		outputDoc(docID, fetchResult.Title, fetchResult.URL, fetchResult.Content)
	}

	return nil
}
