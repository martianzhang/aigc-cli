package chat

import (
	"strings"

	"github.com/martianzhang/aigc-cli/internal/skill"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// expandUserInput applies /skill:<name> expansion before @file mentions, so
// arguments after the skill name can still reference local files.
func expandUserInput(text string, deny []string) (string, []types.ChatImage, []string) {
	if strings.HasPrefix(text, "/skill:") {
		skills, diags := discoverSkills()
		logSkillDiagnostics(diags)
		if expanded, ok := skill.ExpandCommand(skills, text); ok {
			text = expanded
		}
	}
	return expandMentions(text, deny)
}
