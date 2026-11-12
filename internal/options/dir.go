package options

import (
	"os"
	"path/filepath"
)

// ConfigDir returns the aigc-cli configuration directory (~/.config/aigc-cli).
func ConfigDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ".config/aigc-cli"
	}
	return filepath.Join(home, ".config", "aigc-cli")
}

// WatermarkDir returns the learned-watermark directory under the config dir.
func WatermarkDir() string {
	return filepath.Join(ConfigDir(), "watermark")
}
