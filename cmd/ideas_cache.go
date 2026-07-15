package cmd

import (
	"os"
	"path/filepath"

	"github.com/martianzhang/apimart-cli/internal/types"
)

// ideasDir returns the default directory for ideas data: ~/.config/aigc-cli/.
func ideasDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "aigc-cli"), nil
}

// resolveIdeasDataPath returns the path to an existing external ideas.json.
// Order: config ideas.data_path → ~/.config/aigc-cli/ideas/ideas.json → empty.
func resolveIdeasDataPath(cfg *types.Config) string {
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

// ideasDataSavePath returns the path where ideas.json should be saved.
// Unlike resolveIdeasDataPath, this does NOT check file existence.
func ideasDataSavePath(cfg *types.Config) string {
	if cfg != nil && cfg.Ideas != nil && cfg.Ideas.DataPath != "" {
		return cfg.Ideas.DataPath
	}
	dir, err := ideasDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "ideas", "ideas.json")
}
