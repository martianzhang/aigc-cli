package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadConfirmToolsNilVsEmpty locks the distinction chat confirmation relies
// on: an absent key means "use the default patterns", while an explicit []
// means "disable confirmation".
func TestLoadConfirmToolsNilVsEmpty(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantNil bool
		want    []string
	}{
		{
			name:    "absent",
			yaml:    "defaults:\n  chat:\n    model: test\n",
			wantNil: true,
		},
		{
			name: "explicit empty",
			yaml: "defaults:\n  chat:\n    confirm_tools: []\n",
		},
		{
			name: "patterns",
			yaml: "defaults:\n  chat:\n    confirm_tools: ['^bash\\b', '^(write|edit) .*\\.env$']\n",
			want: []string{`^bash\b`, `^(write|edit) .*\.env$`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Defaults == nil || cfg.Defaults.Chat == nil {
				t.Fatal("defaults.chat missing from parsed config")
			}
			got := cfg.Defaults.Chat.ConfirmTools
			if tt.wantNil {
				if got != nil {
					t.Fatalf("ConfirmTools = %#v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("ConfirmTools = nil, want non-nil (empty or patterns)")
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ConfirmTools = %#v, want %#v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("ConfirmTools[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
