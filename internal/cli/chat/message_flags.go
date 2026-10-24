package chat

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// messageValues backs --message and its --prompt/-p alias. Both flags share one
// Value instance so mixing them preserves command-line order: pflag's
// StringArray replaces its slice on the first Set per flag, which would
// silently drop earlier values if each flag owned a separate value object.
type messageValues struct {
	target *[]string
}

func (m *messageValues) Set(val string) error {
	*m.target = append(*m.target, val)
	return nil
}

func (m *messageValues) String() string {
	// Return "" while empty so pflag treats the default as zero and does not
	// render "(default [])" in --help. pflag only special-cases its own
	// *stringArrayValue concrete type, not custom Values.
	if m.target == nil || len(*m.target) == 0 {
		return ""
	}
	return "[" + strings.Join(*m.target, ",") + "]"
}

func (m *messageValues) Type() string { return "stringArray" }

func (m *messageValues) Append(val string) error { return m.Set(val) }

func (m *messageValues) Replace(vals []string) error {
	*m.target = append([]string(nil), vals...)
	return nil
}

func (m *messageValues) GetSlice() []string {
	if m.target == nil {
		return nil
	}
	return *m.target
}

// addMessageFlags registers the canonical --message flag and its --prompt/-p
// alias on f. Both write to the same slice.
func addMessageFlags(f *pflag.FlagSet) {
	v := &messageValues{target: &chatMessages}
	f.Var(v, "message", "User message (repeatable; an existing file path is read as the message)")
	f.VarP(v, "prompt", "p", "Alias for --message (repeatable; an existing file path is read)")
}

// messageFlagChanged reports whether either --message or its --prompt alias was
// supplied.
func messageFlagChanged(cmd *cobra.Command) bool {
	return options.HasFlagChanged(cmd, "message") || options.HasFlagChanged(cmd, "prompt")
}
