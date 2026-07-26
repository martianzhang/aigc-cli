package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/spf13/cobra"
	"golang.org/x/net/html"
)

var (
	kbMapLimit      int
	kbMapSameDomain bool
	kbMapDryRun     bool
)

var kbMapCmd = &cobra.Command{
	Use:   "map <url>",
	Short: "Discover URLs from a page and batch fetch",
	Long: `Fetch a web page, discover all links, and add them to the knowledge base.

By default only follows links on the same domain. Use --no-same-domain to
allow cross-domain links. Use --limit to cap the number of URLs to fetch.

Example:
  aigc-cli kb map https://go.dev/doc/            # discover + fetch all doc pages
  aigc-cli kb map https://example.com --dry-run   # just list URLs, don't fetch`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		rawURL := args[0]
		if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
			rawURL = "https://" + rawURL
		}

		u, err := url.Parse(rawURL)
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}

		// Fetch the page
		fmt.Fprintf(os.Stderr, "Fetching: %s\n", rawURL)
		resp, err := http.Get(rawURL)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", rawURL, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
		}

		// Parse HTML and extract links
		doc, err := html.Parse(resp.Body)
		if err != nil {
			return fmt.Errorf("parse html: %w", err)
		}

		baseDomain := u.Host
		seen := make(map[string]bool)
		var links []string

		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "a" {
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						href := attr.Val
						// Resolve relative URLs
						abs, err := resolveURL(rawURL, href)
						if err != nil || abs == "" {
							continue
						}
						if seen[abs] {
							continue
						}
						seen[abs] = true

						// Filter by domain
						if kbMapSameDomain {
							parsed, err := url.Parse(abs)
							if err != nil || parsed.Host != baseDomain {
								continue
							}
						}

						// Skip anchors and non-HTTP
						if strings.HasPrefix(abs, "#") || strings.Contains(abs, "mailto:") {
							continue
						}

						links = append(links, abs)
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)

		if len(links) == 0 {
			fmt.Fprintln(os.Stderr, "No discoverable links found.")
			return nil
		}

		if kbMapLimit > 0 && len(links) > kbMapLimit {
			links = links[:kbMapLimit]
		}

		fmt.Fprintf(os.Stderr, "Discovered %d link(s)\n", len(links))

		if kbMapDryRun {
			for _, link := range links {
				fmt.Println(link)
			}
			return nil
		}

		// Batch fetch
		store, err := openKBStore()
		if err != nil {
			return err
		}
		defer store.Close()

		chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
		embedder := knowledge.NewHashEmbedder(384)
		project := detectProject(cmd)

		for _, link := range links {
			fmt.Fprintf(os.Stderr, "  Fetching: %s\n", link)
			time.Sleep(500 * time.Millisecond)

			existingDocID, err := findURLInStore(store, link)
			if err == nil && existingDocID != "" {
				// Already exists, read and output
				fmt.Fprintf(os.Stderr, "    Already in KB\n")
				if content := tryReadDoc(existingDocID); content != "" {
					outputDoc(existingDocID, link, link, content)
				}
				continue
			}

			result, fetchErr := knowledge.FetchURL(link)
			if fetchErr != nil {
				fmt.Fprintf(os.Stderr, "    Error: %v\n", fetchErr)
				continue
			}

			docID := knowledge.Checksum(result.Content)
			existing, _ := store.GetDocument(docID)
			if existing != nil {
				fmt.Fprintf(os.Stderr, "    Already in KB (same content)\n")
				if content := tryReadDoc(docID); content != "" {
					outputDoc(docID, result.Title, result.URL, content)
				}
				continue
			}

			doc := &knowledge.Document{
				ID:       docID,
				URL:      result.URL,
				Title:    result.Title,
				Project:  project,
				Size:     result.Size,
				Checksum: docID,
			}
			if err := store.SaveDocument(doc); err != nil {
				fmt.Fprintf(os.Stderr, "    Error saving: %v\n", err)
				continue
			}
			knowledge.SaveDocFile(kbBaseDir, project, docID, result.Title, result.Content)

			rawChunks := chunker.Chunk(result.Content)
			embeddings := make([]knowledge.Embedding, len(rawChunks))
			for i, c := range rawChunks {
				emb, err := embedder.Embed(c.Content)
				if err != nil {
					continue
				}
				embeddings[i] = emb
			}
			if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
				fmt.Fprintf(os.Stderr, "    Error saving chunks: %v\n", err)
				continue
			}

			fmt.Fprintf(os.Stderr, "    Added: %s\n", result.Title)
			outputDoc(docID, result.Title, result.URL, result.Content)
		}

		return nil
	},
}

func init() {
	kbMapCmd.Flags().IntVarP(&kbMapLimit, "limit", "n", 3, "Max URLs to fetch (0 = unlimited)")
	kbMapCmd.Flags().BoolVar(&kbMapSameDomain, "same-domain", true, "Only follow links on the same domain")
	kbMapCmd.Flags().BoolVar(&kbMapDryRun, "dry-run", false, "Only list discovered URLs, don't fetch")
	kbCmd.AddCommand(kbMapCmd)
}

// resolveURL resolves a possibly-relative URL against a base.
func resolveURL(base, href string) (string, error) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
		return "", nil
	}
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return baseURL.ResolveReference(ref).String(), nil
}

// findURLInStore checks if a URL already exists in the store and returns its doc ID.
func findURLInStore(store *knowledge.Store, targetURL string) (string, error) {
	rows, err := store.DB().Query("SELECT id FROM documents WHERE url=?", targetURL)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if rows.Next() {
		var id string
		rows.Scan(&id)
		return id, nil
	}
	return "", nil
}
