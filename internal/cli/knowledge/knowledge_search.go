package knowledge

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/types"
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
	Example: `  aigc-cli kb search "golang context timeout"
  aigc-cli kb search "golang context timeout" --local
  aigc-cli kb search "vector database" --provider duckduckgo`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		verbose := options.Shared.Verbose

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

func registerSearchProviders(router *search.Router) error {
	if providers := options.WebSearchConfig(); providers != nil {
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

func init() {
	kbSearchCmd.Flags().StringVar(&kbSearchProviderFlag, "provider", "", "Search provider: duckduckgo, firecrawl (overrides config defaults)")
	kbSearchCmd.Flags().BoolVar(&kbSearchSaveFlag, "auto-save", true, "Save web results to knowledge base")
	kbSearchCmd.Flags().BoolVar(&kbSearchLocalFlag, "local", false, "Also search local knowledge base")
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

func resolveSearchProvider(cmd *cobra.Command) string {
	if cmd.Flags().Changed("provider") {
		v, _ := cmd.Flags().GetString("provider")
		if v != "" {
			return v
		}
	}
	if cfg := options.KnowledgeDefaults(); cfg != nil && cfg.SearchProvider != "" {
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
