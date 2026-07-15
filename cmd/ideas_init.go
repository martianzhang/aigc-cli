package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/ideas"
)

const ideasDataURL = "https://raw.githubusercontent.com/martianzhang/apimart-cli/refs/heads/main/docs/ideas.json"

// ideasInitCmd represents the `aigc-cli ideas init` subcommand.
var ideasInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download and index ideas data for fast search",
	SilenceUsage: true,
	Long: `Download the AI image prompt ideas dataset and build a search index.

Downloads ideas.json from GitHub, computes a checksum, and imports it into
a SQLite database with an inverted index for fast search.

Uses local ideas.json if already present (skips download).
Provide --from-file to use a custom JSON file.

The database is saved alongside ideas.json with a .db extension.

Proxy settings from config.yaml, env vars (HTTP_PROXY), or --http-proxy flag
are automatically respected.`,
	RunE: runIdeasInit,
}

var (
	ideasForce    bool
	ideasFromFile string
)

func runIdeasInit(cmd *cobra.Command, args []string) error {
	targetPath := ideasDataSavePath(shared.Cfg)
	if targetPath == "" {
		dir, err := ideasDir()
		if err != nil {
			return fmt.Errorf("cannot determine ideas data directory: %w", err)
		}
		targetPath = filepath.Join(dir, "ideas", "ideas.json")
	}
	dbPath := ideas.DBPathFromJSON(targetPath)

	// Determine data source.
	var rawData []byte
	var source string

	if ideasFromFile != "" {
		data, err := os.ReadFile(ideasFromFile)
		if err != nil {
			return fmt.Errorf("cannot read --from-file %s: %w", ideasFromFile, err)
		}
		rawData = data
		source = ideasFromFile
	} else if data, err := os.ReadFile(targetPath); err == nil {
		rawData = data
		source = targetPath + " (local)"
		fmt.Printf("Found local ideas.json (%d MB).\n", len(rawData)/1024/1024)
	} else {
		fmt.Printf("Downloading ideas data from GitHub...\n")
		client := &http.Client{
			Timeout:   120 * time.Second,
			Transport: http.DefaultClient.Transport,
		}
		if client.Transport == nil {
			client.Transport = http.DefaultTransport
		}
		resp, err := client.Get(ideasDataURL)
		if err != nil {
			return fmt.Errorf("download failed: %w\n  Try downloading ideas.json manually and using --from-file", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download failed: HTTP %d\n  URL: %s", resp.StatusCode, ideasDataURL)
		}
		rawData, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
		source = "GitHub"
	}

	// Compute source hash.
	sourceHash := ideas.SourceHash(rawData)

	// Check if DB is already up-to-date.
	if !ideasForce {
		if ideas.DBExists(dbPath) {
			db, err := ideas.OpenDB(dbPath)
			if err == nil {
				ok, checkErr := ideas.CheckSourceHash(db, sourceHash)
				db.Close()
				if checkErr == nil && ok {
					fmt.Printf("Ideas database is already up-to-date (%s).\n", dbPath)
					fmt.Printf("  Use --force to rebuild.\n")
					// Ensure ideas.json exists.
					if _, err := os.Stat(targetPath); os.IsNotExist(err) {
						dir := filepath.Dir(targetPath)
						os.MkdirAll(dir, 0755)
						os.WriteFile(targetPath, rawData, 0644)
					}
					return nil
				}
			}
		}
	}

	// Validate JSON.
	var entries []ideas.IdeaEntry
	if err := json.Unmarshal(rawData, &entries); err != nil {
		return fmt.Errorf("ideas data is corrupted (invalid JSON): %w", err)
	}
	fmt.Printf("Loaded %d prompt entries from %s.\n", len(entries), source)

	// Save ideas.json.
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", dir, err)
	}
	if err := os.WriteFile(targetPath, rawData, 0644); err != nil {
		return fmt.Errorf("cannot save %s: %w", targetPath, err)
	}
	fmt.Printf("Saved to %s\n", targetPath)

	// Build SQLite database with inverted index.
	fmt.Printf("Building search index...\n")
	start := time.Now()
	if err := ideas.InitDB(dbPath, entries, sourceHash); err != nil {
		return fmt.Errorf("failed to build search index: %w", err)
	}
	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Printf("Search index built at %s (%s, %d entries)\n", dbPath, elapsed, len(entries))

	return nil
}

func init() {
	ideasCmd.AddCommand(ideasInitCmd)
	ideasInitCmd.Flags().BoolVar(&ideasForce, "force", false, "Rebuild index even if up-to-date")
	ideasInitCmd.Flags().StringVar(&ideasFromFile, "from-file", "", "Use local ideas.json file instead of downloading")
}
