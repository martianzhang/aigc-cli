package cmd

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var kbPruneCheckURLs bool

var kbPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove duplicates and dead URLs",
	Long: `Clean up the knowledge base:
  - Removes duplicate documents (same checksum)
  - With --check-urls, checks URL-based docs for 404 and removes dead ones`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openKBStore()
		if err != nil {
			return err
		}
		defer store.Close()

		// Check for duplicate checksums
		rows, err := store.DB().Query(`
			SELECT checksum, COUNT(*) as cnt, MIN(id) as keep_id
			FROM documents
			WHERE checksum != ''
			GROUP BY checksum
			HAVING cnt > 1`)
		if err != nil {
			return fmt.Errorf("find duplicates: %w", err)
		}

		type dup struct {
			checksum string
			count    int
			keepID   string
		}
		var dups []dup
		for rows.Next() {
			var d dup
			if err := rows.Scan(&d.checksum, &d.count, &d.keepID); err != nil {
				continue
			}
			dups = append(dups, d)
		}
		rows.Close()

		removed := 0
		for _, d := range dups {
			// Delete all except the one to keep
			res, err := store.DB().Exec(
				"DELETE FROM documents WHERE checksum=? AND id!=?", d.checksum, d.keepID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  Error cleaning dup %s: %v\n", d.checksum[:12], err)
				continue
			}
			n, _ := res.RowsAffected()
			removed += int(n)
			fmt.Fprintf(os.Stderr, "  Removed %d duplicate(s) of %s\n", n, d.checksum[:12])
		}

		fmt.Fprintf(os.Stderr, "Duplicates removed: %d\n", removed)

		// Check URLs if requested
		if kbPruneCheckURLs {
			docs, err := store.ListDocuments(1000, 0, "")
			if err != nil {
				return fmt.Errorf("list docs: %w", err)
			}

			client := &http.Client{Timeout: 10 * time.Second}
			for _, d := range docs {
				if d.URL == "" {
					continue
				}
				fmt.Fprintf(os.Stderr, "  Checking: %s\n", d.URL[:minInt(60, len(d.URL))])
				resp, err := client.Head(d.URL)
				if err != nil || resp.StatusCode >= 400 {
					code := 0
					if resp != nil {
						code = resp.StatusCode
					}
					fmt.Fprintf(os.Stderr, "    Dead (%d): %s\n    Removing...\n", code, d.Title)
					if err := store.DeleteDocument(d.ID); err != nil {
						fmt.Fprintf(os.Stderr, "    Error: %v\n", err)
					}
				}
				if resp != nil {
					resp.Body.Close()
				}
				time.Sleep(200 * time.Millisecond)
			}
		}

		return nil
	},
}

func init() {
	kbPruneCmd.Flags().BoolVar(&kbPruneCheckURLs, "check-urls", false, "Check URL-based docs for 404 and remove dead ones")
	kbCmd.AddCommand(kbPruneCmd)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
