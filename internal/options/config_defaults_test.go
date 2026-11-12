package options

import (
	"testing"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestChatBashTimeout(t *testing.T) {
	orig := Shared.Cfg
	t.Cleanup(func() { Shared.Cfg = orig })

	cases := []struct {
		name string
		cfg  *types.Config
		want time.Duration
	}{
		{
			name: "no config uses built-in",
			cfg:  nil,
			want: 300 * time.Second,
		},
		{
			name: "unset uses built-in",
			cfg:  &types.Config{Defaults: &types.ConfigDefaults{Chat: &types.ChatDefaults{}}},
			want: 300 * time.Second,
		},
		{
			name: "configured wins",
			cfg: &types.Config{Defaults: &types.ConfigDefaults{Chat: &types.ChatDefaults{
				Bash: &types.ChatBashDefaults{Timeout: 900},
			}}},
			want: 15 * time.Minute,
		},
		{
			name: "zero falls back",
			cfg: &types.Config{Defaults: &types.ConfigDefaults{Chat: &types.ChatDefaults{
				Bash: &types.ChatBashDefaults{Timeout: 0},
			}}},
			want: 300 * time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Shared.Cfg = tc.cfg
			if got := ChatBashTimeout(); got != tc.want {
				t.Errorf("ChatBashTimeout() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestChatBashTimeoutFromConfigFile locks the YAML path, so the
// mapstructure/yaml tags on ChatBashDefaults stay loadable.
func TestChatBashTimeoutFromConfigFile(t *testing.T) {
	orig := Shared.Cfg
	t.Cleanup(func() { Shared.Cfg = orig })
	Shared.Cfg = nil

	useConfig(t, "defaults:\n  chat:\n    bash:\n      timeout: 900\n")

	if got := ChatBashTimeout(); got != 15*time.Minute {
		t.Errorf("ChatBashTimeout() = %v, want 15m from defaults.chat.bash.timeout", got)
	}
}
