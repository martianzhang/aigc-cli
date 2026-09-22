package ideas

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/ideas"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const dataURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1/ideas.json"

func newInitCommand(deps func() Deps) *cobra.Command {
	return &cobra.Command{
		Use:          "init",
		Short:        "Download ideas data",
		SilenceUsage: true,
		Long: `Download the AI image prompt ideas dataset.

The data is saved to ~/.config/aigc-cli/ideas/ideas.json (or the configured ideas.data_path).

Proxy settings from config.yaml, env vars (HTTP_PROXY), or --http-proxy flag
are automatically respected.`,
		Example: `  aigc-cli ideas init`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(deps())
		},
	}
}

// --- data path helpers ---

func ideasDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "aigc-cli"), nil
}

func resolveDataPath(cfg *types.Config) string {
	if cfg != nil && cfg.Ideas != nil && cfg.Ideas.DataPath != "" {
		return cfg.Ideas.DataPath
	}
	dir, err := ideasDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(dir, "ideas", "ideas.json")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

func dataSavePath(cfg *types.Config) string {
	if cfg != nil && cfg.Ideas != nil && cfg.Ideas.DataPath != "" {
		return cfg.Ideas.DataPath
	}
	dir, err := ideasDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ideas", "ideas.json")
}

func runInit(d Deps) error {
	targetPath := dataSavePath(d.Cfg)
	if targetPath == "" {
		dir, err := ideasDir()
		if err != nil {
			return fmt.Errorf("cannot determine ideas data directory: %w", err)
		}
		targetPath = filepath.Join(dir, "ideas", "ideas.json")
	}

	if _, err := os.Stat(targetPath); err == nil {
		fmt.Fprintf(os.Stderr, "%s already exists.\n  To re-download the latest data, delete it first:\n    rm %s\n  Then run 'aigc-cli ideas init' again.\n", targetPath, targetPath)
		return fmt.Errorf("ideas data already exists")
	}

	fmt.Printf("Downloading ideas data from GitHub...\n")

	client := httpClient()

	resp, err := client.Get(dataURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d\n  URL: %s", resp.StatusCode, dataURL)
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

func httpClient() *http.Client {
	client := &http.Client{
		Timeout:   120 * time.Second,
		Transport: http.DefaultClient.Transport,
	}
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}
	return client
}
