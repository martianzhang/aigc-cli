package cmd

import (
	"os"
	"path/filepath"

	"github.com/martianzhang/apimart-cli/internal/ideas"
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

// resolveIdeasDBPath returns the path to the ideas SQLite database.
// Derived from the JSON data path (same path, .db extension instead of .json).
func resolveIdeasDBPath(cfg *types.Config) string {
	jsonPath := ideasDataSavePath(cfg)
	return ideas.DBPathFromJSON(jsonPath)
}
