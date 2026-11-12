package options

import (
	"os"
	"path/filepath"
	"testing"
)

// useConfig points Shared at a throwaway config file for the duration of a test.
func useConfig(t *testing.T, yaml string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	prevFile, prevFlag := Shared.CfgFile, Shared.TimeoutFlag
	Shared.CfgFile = path
	Shared.TimeoutFlag = 0
	t.Cleanup(func() {
		Shared.CfgFile, Shared.TimeoutFlag = prevFile, prevFlag
	})
}
