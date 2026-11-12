package knowledge

import (
	"fmt"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/ui"
	"github.com/spf13/cobra"
)

var (
	kbFindLimit int
	kbFindShow  bool
)

var kbFindCmd = &cobra.Command{
	Use:   "find <query>",
	Short: "Search knowledge base (returns matching documents)",
	Long: `Search the knowledge base. Results are grouped by document with a summary.

Use --show to display full chunk content inline.`,
	Example: `  aigc-cli kb find "install steps"
  aigc-cli kb find "install steps" --show
  aigc-cli kb find "install steps" -n 5`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openKBStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer func() { _ = store.Close() }()

		project := resolveSearchProject(cmd)
		query := args[0]
		results, err := store.Search(query, kbFindLimit*3, project)
		if err != nil {
			return fmt.Errorf("search: %w", err)
		}

		type docResult struct {
			doc     knowledge.Document
			score   float64
			chunks  []knowledge.Chunk
			summary string
		}
		docMap := make(map[string]*docResult)
		for _, r := range results {
			d, ok := docMap[r.Document.ID]
			if !ok {
				docMap[r.Document.ID] = &docResult{
					doc:   r.Document,
					score: r.Score,
				}
				d = docMap[r.Document.ID]
			}
			if r.Score > d.score {
				d.score = r.Score
			}
			d.chunks = append(d.chunks, r.Chunk)
		}

		docs := make([]*docResult, 0, len(docMap))
		for _, d := range docMap {
			// Build summary from the first chunk's content
			if len(d.chunks) > 0 {
				d.summary = firstParagraph(d.chunks[0].Content)
			}
			docs = append(docs, d)
		}
		sort.Slice(docs, func(i, j int) bool {
			return docs[i].score > docs[j].score
		})

		if kbFindLimit > 0 && len(docs) > kbFindLimit {
			docs = docs[:kbFindLimit]
		}

		if len(docs) == 0 {
			ui.Eprintln("No results found.")
			return nil
		}

		ui.Eprintf("Found %d matching document(s)\n", len(docs))

		for i, d := range docs {
			source := d.doc.URL
			if source == "" {
				source = d.doc.FilePath
			}

			ui.Println("\n---")
			ui.Printf("### [%d] %s\n", i+1, d.doc.Title)
			ui.Printf("**ID:** `%s`  \n", knowledge.ShortID(d.doc.ID))
			ui.Printf("**Source:** %s  \n", source)
			ui.Printf("**Score:** %.4f (%d chunk(s))  \n", d.score, len(d.chunks))
			ui.Printf("**Show:** `aigc-cli kb show %s`  \n", knowledge.ShortID(d.doc.ID))

			if d.summary != "" {
				ui.Println()
				ui.Println(d.summary)
			}
		}
		ui.Println("\n---")

		if !kbFindShow {
			return nil
		}

		// --show: output full content for each document
		for _, d := range docs {
			ui.Printf("\n## %s (`%s`)\n\n", d.doc.Title, knowledge.ShortID(d.doc.ID))
			for _, c := range d.chunks {
				if c.Heading != "" {
					ui.Printf("### %s\n\n", strings.TrimPrefix(c.Heading, "# "))
				}
				ui.Println(c.Content)
				ui.Println()
			}
			ui.Println("---")
		}
		return nil
	},
}

func init() {
	kbFindCmd.Flags().IntVarP(&kbFindLimit, "limit", "n", 10, "Max documents")
	kbFindCmd.Flags().BoolVarP(&kbFindShow, "show", "s", false, "Display full content inline")
}

func firstParagraph(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Take up to 300 chars, break at sentence end or newline
	maxLen := 300
	if len(s) <= maxLen {
		return s
	}
	// Try to break at a newline within range
	if idx := strings.Index(s[:maxLen], "\n"); idx > 50 {
		return s[:idx]
	}
	// Try to break at a period
	if idx := strings.LastIndex(s[:maxLen], ". "); idx > 50 {
		return s[:idx+1]
	}
	return s[:maxLen] + "..."
}
