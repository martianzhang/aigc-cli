package knowledge

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/ui"
	"github.com/martianzhang/aigc-cli/internal/wait"
)

var kbPruneCheckURLs bool

var kbPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove duplicates and dead URLs",
	Long: `Clean up the knowledge base:
  - Removes duplicate documents (same checksum)
  - With --check-urls, checks URL-based docs for 404 and removes dead ones`,
	Example: `  aigc-cli kb prune
  aigc-cli kb prune --check-urls`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openKBStore()
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()

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
		defer func() { _ = rows.Close() }()

		type dup struct {
			checksum string
			count    int
			keepID   string
		}
		var dups []dup
		for rows.Next() {
			var d dup
			if err := rows.Scan(&d.checksum, &d.count, &d.keepID); err != nil {
				return fmt.Errorf("scan duplicate: %w", err)
			}
			dups = append(dups, d)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate duplicates: %w", err)
		}

		removed := 0
		for _, d := range dups {
			// Delete all except the one to keep
			res, err := store.DB().Exec(
				"DELETE FROM documents WHERE checksum=? AND id!=?", d.checksum, d.keepID)
			if err != nil {
				ui.Eprintf("  Error cleaning dup %s: %v\n", knowledge.ShortID(d.checksum), err)
				continue
			}
			n, _ := res.RowsAffected()
			removed += int(n)
			ui.Eprintf("  Removed %d duplicate(s) of %s\n", n, knowledge.ShortID(d.checksum))
		}

		ui.Eprintf("Duplicates removed: %d\n", removed)

		// Check URLs if requested
		if kbPruneCheckURLs {
			docs, err := store.ListDocuments(1000, 0, "")
			if err != nil {
				return fmt.Errorf("list docs: %w", err)
			}

			hc := &http.Client{Timeout: 10 * time.Second}
			for _, d := range docs {
				if d.URL == "" {
					continue
				}
				ui.Eprintf("  Checking: %s\n", d.URL[:min(60, len(d.URL))])
				resp, err := hc.Head(d.URL)
				if err != nil || resp.StatusCode >= 400 {
					code := 0
					if resp != nil {
						code = resp.StatusCode
					}
					ui.Eprintf("    Dead (%d): %s\n    Removing...\n", code, d.Title)
					if err := store.DeleteDocument(d.ID); err != nil {
						ui.Eprintf("    Error: %v\n", err)
					}
				}
				if resp != nil {
					resp.Body.Close()
				}
				if err := wait.Sleep(options.Context(), 200*time.Millisecond); err != nil {
					return err
				}
			}
		}

		return nil
	},
}

func init() {
	kbPruneCmd.Flags().BoolVar(&kbPruneCheckURLs, "check-urls", false, "Check URL-based docs for 404 and remove dead ones")
}
