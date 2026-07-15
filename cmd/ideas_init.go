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
	Short:        "Download ideas data",
	SilenceUsage: true,
	Long: `Download the AI image prompt ideas dataset.

The data is saved to ~/.config/aigc-cli/ideas.json (or the configured ideas.data_path).

Proxy settings from config.yaml, env vars (HTTP_PROXY), or --http-proxy flag
are automatically respected.`,
	RunE: runIdeasInit,
}

func runIdeasInit(cmd *cobra.Command, args []string) error {
	targetPath := ideasDataSavePath(shared.Cfg)
	if targetPath == "" {
		dir, err := ideasDir()
		if err != nil {
			return fmt.Errorf("cannot determine ideas data directory: %w", err)
		}
		targetPath = filepath.Join(dir, "ideas.json")
	}

	if _, err := os.Stat(targetPath); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists.\n  To re-download the latest data, delete it first:\n    rm %s\n  Then run 'aigc-cli ideas init' again.\n", targetPath, targetPath)
		return fmt.Errorf("ideas data already exists")
	}

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

	var entries []ideas.IdeaEntry
	if err := json.Unmarshal(rawData, &entries); err != nil {
		return fmt.Errorf("downloaded data is corrupted (invalid JSON): %w", err)
	}
	fmt.Printf("Downloaded %d prompt entries.\n", len(entries))

	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(targetPath, rawData, 0644); err != nil {
		return fmt.Errorf("cannot save %s: %w", targetPath, err)
	}
	fmt.Printf("Saved to %s\n", targetPath)

	return nil
}

func init() {
	ideasCmd.AddCommand(ideasInitCmd)
}
