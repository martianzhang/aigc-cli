package cmd

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/spf13/cobra"
)

var (
	kbSearchProviderFlag string
	kbSearchSaveFlag     = true
	kbSearchLocalFlag    bool
)

var kbSearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Web search and optionally merge local KB results",
	Long: `Search the web using configured search provider and save results to KB.

By default, only searches the web. Use --local to also include local KB results.`,
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

		if kbSearchLocalFlag {
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
		}

		if err := searchWebWithRouter(store, cmd, query, project, verbose); err != nil {
			return err
		}

		return nil
	},
}

// searchWebWithRouter uses the search router or falls back to direct providers.
func searchWebWithRouter(store *knowledge.Store, cmd *cobra.Command, query, project string, verbose bool) error {
	if cmd.Flags().Changed("provider") {
		provider, _ := cmd.Flags().GetString("provider")
		return searchWithProvider(store, cmd, query, project, provider, verbose)
	}

	provider := resolveSearchProvider(cmd)
	return searchWithProvider(store, cmd, query, project, provider, verbose)
}

// searchWithProvider uses router with specified provider or strategy.
func searchWithProvider(store *knowledge.Store, cmd *cobra.Command, query, project string, provider string, verbose bool) error {
	qStore, err := search.NewQuotaStore(store.DB())
	if err != nil {
		return fmt.Errorf("quota store: %w", err)
	}

	router := search.NewRouter(qStore)

	if err := registerSearchProviders(router); err != nil {
		return err
	}

	strategy, preferred := resolveSearchStrategy(provider)

	searchResult, err := router.Search(query, 3, strategy, preferred)
	if err != nil {
		return err
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Using provider: %s\n", searchResult.Provider)
	}

	return fetchSearchResults(store, cmd, searchResult.Results, query, project, verbose)
}

func registerSearchProviders(router *search.Router) error {
	if providers := webSearchConfig(); providers != nil {
		for name, cfg := range providers {
			info := search.ConfigFromTypes(map[string]*types.WebSearchProvider{name: cfg})[name]
			p, err := search.NewProviderFromConfig(cfg)
			if err != nil {
				continue
			}
			router.Register(name, p, info)
		}
	}

	if _, exists := router.GetProvider("duckduckgo"); !exists {
		router.Register("duckduckgo", search.NewDDGProvider(), &search.ProviderInfo{
			Type:   "duckduckgo",
			Tags:   []string{"free"},
			Weight: 1,
		})
	}
	return nil
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

		if shouldAutoSave() {
			doc := &knowledge.Document{
				ID: docID, URL: fetchResult.URL, Title: fetchResult.Title,
				Project: project, Size: fetchResult.Size, Checksum: docID,
			}
			if err := store.SaveDocument(doc); err != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "    Error saving: %v\n", err)
				}
			} else {
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
				}
				if verbose {
					fmt.Fprintf(os.Stderr, "    Saved: %s\n", fetchResult.Title)
				}
			}
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
	if cfg := knowledgeDefaults(); cfg != nil && cfg.SearchProvider != "" {
		return cfg.SearchProvider
	}
	return "auto"
}

func resolveSearchStrategy(provider string) (string, []string) {
	switch strings.ToLower(provider) {
	case "auto", "":
		return "auto", nil
	case "free", "cheap":
		return "cheap", nil
	case "quality":
		return "quality", nil
	default:
		return "manual", []string{provider}
	}
}

func init() {
	kbSearchCmd.Flags().StringVar(&kbSearchProviderFlag, "provider", "", "Search provider: duckduckgo, firecrawl (overrides config defaults)")
	kbSearchCmd.Flags().BoolVar(&kbSearchSaveFlag, "auto-save", true, "Save web results to knowledge base")
	kbSearchCmd.Flags().BoolVar(&kbSearchLocalFlag, "local", false, "Also search local knowledge base")
	kbCmd.AddCommand(kbSearchCmd)
}

func outputDoc(docID, title, url, content string) {
	fmt.Println("---")
	fmt.Printf("id: %s\n", docID[:12])
	fmt.Printf("title: %s\n", title)
	fmt.Printf("url: %s\n", url)
	if host := extractHost(url); host != "" {
		fmt.Printf("source: %s\n", host)
	}
	fmt.Println(content)
}

// extractHost returns the hostname from a URL, or empty string.
func extractHost(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Host
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

	if !verbose {
		return
	}

	fmt.Printf("From knowledge base:\n\n")
	for i, d := range docs {
		fmt.Printf("[%d] %s\n", i+1, d.title)
		fmt.Printf("    id: %s | source: %s | score: %.4f (%d chunk(s))\n", d.docID, d.source, d.score, d.nchunk)
	}
	fmt.Println()
}
