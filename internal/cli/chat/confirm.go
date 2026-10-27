package chat

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbletea"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// confirmDecision is the user's answer to a tool confirmation prompt.
type confirmDecision int

const (
	confirmDeny confirmDecision = iota
	confirmOnce
	confirmAlways
	// confirmSuggest denies the call but carries the user's own instruction back
	// to the model, so it can retry with a different approach in the same turn
	// instead of dead-ending on a bare denial.
	confirmSuggest
)

// confirmReply is the user's answer to a confirmRequest. feedback is only set
// for confirmSuggest and holds the text typed in the prompt.
type confirmReply struct {
	decision confirmDecision
	feedback string
}

// defaultConfirmPatterns keeps the safe default: the three tools that can
// change the machine or the project ask once per call. Users can replace this
// with any regex list via defaults.chat.confirm_tools, or set the list to []
// to turn confirmation off entirely.
var defaultConfirmPatterns = []string{
	`^bash\b`,
	`^write\b`,
	`^edit\b`,
}

// deniedSummary is the toolDone summary used when the user rejects a call.
const deniedSummary = "denied by user"

// deniedToolResult is the tool result the model sees when the user rejects a
// call. A suggestion turns the rejection into guidance the model can act on
// immediately, rather than a dead end that needs a second user message.
func deniedToolResult(r confirmReply) string {
	if r.decision == confirmSuggest {
		if text := strings.TrimSpace(r.feedback); text != "" {
			return "The user rejected this tool call and asked for a different approach: " + text
		}
	}
	return "User denied this tool call."
}

// deniedSummaryFor is the one-line tool-card summary for a rejection.
func deniedSummaryFor(r confirmReply) string {
	if r.decision == confirmSuggest {
		if text := strings.TrimSpace(r.feedback); text != "" {
			return deniedSummary + " · " + clipString(text, 40)
		}
	}
	return deniedSummary
}

// confirmPolicy decides whether a tool call needs user confirmation. Patterns
// are matched against "<tool> <target>" (see confirmMatchText). Tools the user
// chose to "always allow" for this session are skipped.
type confirmPolicy struct {
	patterns []*regexp.Regexp

	mu     sync.Mutex
	always map[string]bool
}

// newConfirmPolicy compiles defaults.chat.confirm_tools.
//
//   - nil (key absent)  -> default patterns (bash/write/edit)
//   - empty (explicit []) -> no confirmation at all
//   - non-empty         -> custom regex list
//
// Invalid regexes are reported as an error rather than silently disabling the
// safety check.
func newConfirmPolicy(cfg *types.ChatDefaults) (*confirmPolicy, error) {
	var raw []string
	switch {
	case cfg == nil || cfg.ConfirmTools == nil:
		raw = defaultConfirmPatterns
	default:
		raw = cfg.ConfirmTools
	}

	p := &confirmPolicy{always: map[string]bool{}}
	for _, expr := range raw {
		re, err := regexp.Compile(expr)
		if err != nil {
			return nil, fmt.Errorf("defaults.chat.confirm_tools: invalid regex %q: %w", expr, err)
		}
		p.patterns = append(p.patterns, re)
	}
	return p, nil
}

// needs reports whether the tool call must be confirmed. The authoritative
// check is always run immediately before execution, so a tool that was
// "always allowed" earlier in the session skips the prompt.
func (p *confirmPolicy) needs(tool, argsJSON string) bool {
	if p == nil || len(p.patterns) == 0 {
		return false
	}
	p.mu.Lock()
	allowed := p.always[tool]
	p.mu.Unlock()
	if allowed {
		return false
	}
	target := confirmMatchText(tool, argsJSON)
	for _, re := range p.patterns {
		if re.MatchString(target) {
			return true
		}
	}
	return false
}

// allowAlways remembers a tool for the rest of this process. It is scoped to
// the session: nothing is persisted.
func (p *confirmPolicy) allowAlways(tool string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.always[tool] = true
	p.mu.Unlock()
}

// confirmMatchText builds the canonical string that confirmation patterns are
// matched against:
//
//	bash  -> "bash <full command>"
//	file tools -> "<tool> <filepath>"
//	url tools  -> "<tool> <url>"
//	query tools-> "<tool> <query>"
//	others     -> "<tool> <compact json>"
//
// This lets users write narrowly scoped rules such as:
//
//	^bash .*rm\s+-rf
//	^(write|edit) .*\.env$
//	^generate_image\b
func confirmMatchText(tool, argsJSON string) string {
	args := parseToolArgs(argsJSON)
	target := ""
	switch {
	case tool == "bash":
		target = argString(args, "command")
	case argString(args, "filepath") != "":
		target = argString(args, "filepath")
	case argString(args, "url") != "":
		target = argString(args, "url")
	case argString(args, "query") != "":
		target = argString(args, "query")
	default:
		target = compactJSON(argsJSON)
	}
	if target == "" {
		return tool
	}
	return tool + " " + target
}

// requestToolConfirmation asks the UI to approve a tool call and blocks until
// the user answers or the turn is cancelled. detail carries the full command
// for bash so a multi-line script can be inspected before approval.
func (m *chatModel) requestToolConfirmation(ctx context.Context, prog *tea.Program, turn uint64, name, summary, detail string) confirmReply {
	reply := make(chan confirmReply, 1)
	prog.Send(confirmRequest{turn: turn, name: name, summary: summary, detail: detail, reply: reply})
	select {
	case answer := <-reply:
		return answer
	case <-ctx.Done():
		return confirmReply{decision: confirmDeny}
	}
}

// confirmDetail returns extra context shown in the confirmation prompt. Only
// bash needs it: its header summary is deliberately single-line, but the user
// must be able to read the whole command before approving it.
func confirmDetail(name, argsJSON string) string {
	if name != "bash" {
		return ""
	}
	return argString(parseToolArgs(argsJSON), "command")
}
