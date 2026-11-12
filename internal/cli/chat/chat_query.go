package chat

import (
	"encoding/json"
	"fmt"

	"github.com/martianzhang/aigc-cli/internal/cliutil"
)

// printToolArgs echoes a tool call's arguments to stderr (verbose tool-use
// display). Web fetching/search execution lives in internal/webtool.
func printToolArgs(argsJSON string) {
	if argsJSON == "" || argsJSON == "{}" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		fmt.Fprintf(cliutil.Stderr(), "  %s\r\n", clipString(argsJSON, 120))
		return
	}
	for k, v := range m {
		s := fmt.Sprintf("%v", v)
		fmt.Fprintf(cliutil.Stderr(), "  %s=%s\r\n", k, clipString(s, 80))
	}
}
