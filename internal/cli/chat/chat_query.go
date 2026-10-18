package chat

import (
	"encoding/json"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// printToolArgs echoes a tool call's arguments to stderr (verbose tool-use
// display). Web fetching/search execution lives in internal/webtool.
func printToolArgs(argsJSON string) {
	if argsJSON == "" || argsJSON == "{}" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		raw := argsJSON
		if len(raw) > 120 {
			raw = raw[:120] + "..."
		}
		fmt.Fprintf(options.Stderr(), "  %s\r\n", raw)
		return
	}
	for k, v := range m {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "..."
		}
		fmt.Fprintf(options.Stderr(), "  %s=%s\r\n", k, s)
	}
}
