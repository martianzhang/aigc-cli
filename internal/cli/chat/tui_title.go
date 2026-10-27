package chat

import (
	"github.com/martianzhang/aigc-cli/internal/terminal"
)

// syncTitle mirrors the TUI state into the terminal tab title so a background
// tab still shows what the agent is doing:
//
//	aigc-cli · chat           idle
//	aigc-cli · chat …         streaming the model's reply
//	aigc-cli · chat · tool …  running a tool call
//	aigc-cli · chat · confirm waiting for y/a/n
//
// It runs from View, i.e. after every state change; the reporter drops repeated
// identical titles, so this stays free of terminal chatter.
func (m *chatModel) syncTitle() {
	prog := terminal.Default()
	if !prog.Enabled() {
		return
	}
	stage, busy := chatTitle(m.state, m.pending != nil)
	if busy {
		prog.BusyStage(stage)
		return
	}
	prog.Finish(stage)
}

// chatTitle maps a TUI state to the tab label and whether it is an in-flight
// phase (indeterminate progress indicator) or a settled one.
func chatTitle(state tuiState, pending bool) (stage string, busy bool) {
	if pending {
		return "chat · confirm", true
	}
	switch state {
	case tuiProcessing:
		return "chat", true
	case tuiToolCall:
		return "chat · tool", true
	default:
		return "chat", false
	}
}
