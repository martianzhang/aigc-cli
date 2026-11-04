package agent

import "strings"

const summarizationSystemPrompt = `You are a context summarization assistant. Read the conversation and produce a structured checkpoint summary that another assistant will use to continue the work.

Do NOT continue the conversation. Do NOT answer questions in it. ONLY output the requested summary. Preserve exact file paths, function names, commands and error messages. Write the summary in the same language as the conversation.`

const initialSummaryFormat = `Create a structured context checkpoint using this EXACT format:

## Goal
[What is the user trying to accomplish? Multiple items are fine.]

## Constraints & Preferences
- [Requirements, preferences or constraints mentioned by the user]
- [Or "(none)"]

## Progress
### Done
- [x] [Completed work]

### In Progress
- [ ] [Current work]

### Blocked
- [Blockers, if any]

## Key Decisions
- **[Decision]**: [Brief rationale]

## Next Steps
1. [What should happen next]

## Critical Context
- [Data, examples or references needed to continue]
- [Or "(none)"]

Keep each section concise.`

const updateSummaryRules = `Update the existing structured summary with the new information above. Rules:
- PRESERVE all existing information from the previous summary.
- ADD new progress, decisions and context from the new messages.
- Move completed items from "In Progress" to "Done".
- Update "Next Steps" based on what was accomplished.
- PRESERVE exact file paths, function names, commands and error messages.
- You may remove information that is no longer relevant.

Use the same EXACT format as the previous summary (Goal, Constraints & Preferences, Progress, Key Decisions, Next Steps, Critical Context).`

const splitTurnNote = `The earliest messages in this batch are the beginning of an ongoing turn. Keep the user's current request and the work already done on it explicit so the later retained messages still make sense.`

// buildSummaryPrompt wraps the serialized conversation and instructions in
// tagged plain text. The model receives one user message, not a chat transcript,
// so it does not try to continue the conversation.
func buildSummaryPrompt(conversation, previousSummary, customInstructions string, split bool) string {
	var b strings.Builder
	b.WriteString("<conversation>\n")
	b.WriteString(conversation)
	b.WriteString("\n</conversation>\n\n")
	if previousSummary = strings.TrimSpace(previousSummary); previousSummary != "" {
		b.WriteString("<previous-summary>\n")
		b.WriteString(previousSummary)
		b.WriteString("\n</previous-summary>\n\n")
		b.WriteString(updateSummaryRules)
	} else {
		b.WriteString(initialSummaryFormat)
	}
	if split {
		b.WriteString("\n\n")
		b.WriteString(splitTurnNote)
	}
	if custom := strings.TrimSpace(customInstructions); custom != "" {
		b.WriteString("\n\nAdditional focus from the user: ")
		b.WriteString(custom)
	}
	return b.String()
}
