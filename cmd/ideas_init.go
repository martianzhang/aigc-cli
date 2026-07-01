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
)

const ideasDataURL = "https://raw.githubusercontent.com/martianzhang/apimart-cli/refs/heads/main/docs/ideas.json"

// ideasInitCmd represents the `apimart-cli ideas init` subcommand.
var ideasInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download ideas data and build search index cache",
	SilenceUsage: true,
	Long: `Download the AI image prompt ideas dataset and build the search index cache.

The data is saved to ~/.config/apimart/ideas.json (or the configured ideas.data_path)
and the search index is cached at ~/.config/apimart/ideas.index for fast startup.

Proxy settings from config.yaml, env vars (HTTP_PROXY), or --http-proxy flag
are automatically respected.`,
	RunE: runIdeasInit,
}

func runIdeasInit(cmd *cobra.Command, args []string) error {
	// Determine target path for ideas.json
	targetPath := ideasDataSavePath(shared.Cfg)
	if targetPath == "" {
		dir, err := ideasDir()
		if err != nil {
			return fmt.Errorf("cannot determine ideas data directory: %w", err)
		}
		targetPath = filepath.Join(dir, "ideas.json")
	}

	// Don't re-download if already exists
	if _, err := os.Stat(targetPath); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists.\n  To re-download the latest data, delete it first:\n    rm %s\n  Then run 'apimart-cli ideas init' again.\n", targetPath, targetPath)
		return fmt.Errorf("ideas data already exists")
	}

	// Download — uses http.DefaultClient which inherits proxy from ConfigureDefaultClient
	fmt.Printf("Downloading ideas data from GitHub...\n")

	client := &http.Client{
		Timeout: 120 * time.Second,
		// Inherits proxy transport from http.DefaultClient
		Transport: http.DefaultClient.Transport,
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}

	resp, err := client.Get(ideasDataURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d\n  URL: %s", resp.StatusCode, ideasDataURL)
	}

	rawData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Validate JSON before saving
	var entries []IdeaEntry
	if err := json.Unmarshal(rawData, &entries); err != nil {
		return fmt.Errorf("downloaded data is corrupted (invalid JSON): %w", err)
	}
	fmt.Printf("Downloaded %d prompt entries.\n", len(entries))

	// Ensure directory exists
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", dir, err)
	}

	// Save ideas.json
	if err := os.WriteFile(targetPath, rawData, 0644); err != nil {
		return fmt.Errorf("cannot save %s: %w", targetPath, err)
	}
	fmt.Printf("Saved to %s\n", targetPath)

	// Build and cache search index
	if isIdeasCacheEnabled(shared.Cfg) {
		fmt.Println("Building search index...")
		idx := buildBM25Index(entries)
		hash := computeHash(rawData)
		saveCachedIndex(shared.Cfg, idx, hash)
		fmt.Println("Search index cached.")
	} else {
		fmt.Println("Tip: enable ideas.cache_enabled in config.yaml to cache the search index for faster startup.")
	}

	return nil
}

func init() {
	ideasCmd.AddCommand(ideasInitCmd)
}
