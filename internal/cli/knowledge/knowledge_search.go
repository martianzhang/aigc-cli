package knowledge

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/search"
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

func init() {
	kbSearchCmd.Flags().StringVar(&kbSearchProviderFlag, "provider", "", "Search provider: duckduckgo, firecrawl, brave, doubao, deepseek, exa (overrides config defaults)")
	kbSearchCmd.Flags().BoolVar(&kbSearchSaveFlag, "auto-save", true, "Save web results to knowledge base")
	kbSearchCmd.Flags().BoolVar(&kbSearchLocalFlag, "local", false, "Also search local knowledge base")
}

// searchWebWithRouter runs one explicit provider and ingests its results.
func searchWebWithRouter(store *knowledge.Store, cmd *cobra.Command, query, project string, verbose bool) error {
	provider := resolveSearchProvider(cmd)

	router := search.NewRouterAll(options.WebSearchConfig())

	searchResult, err := router.Search(query, 3, provider, search.Options{})
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
	if p := options.SearchProviderDefault(); p != "" {
		return p
	}
	return "duckduckgo"
}
