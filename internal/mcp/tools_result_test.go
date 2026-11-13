package mcp

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/testutil"
)

func TestWithMCPResultFooter(t *testing.T) {
	f := testutil.AbsPath(t, "/tmp/a.png")
	text := withMCPResultFooter("已保存的图片: "+f, []string{f}, "gpt-image-2", "apimart", "task_1", 0.08, "aigc-cli task task_1")
	for _, want := range []string{
		"已保存的图片",
		"Saved:",
		f,
		"Model: gpt-image-2 (apimart)",
		"Task: task_1",
		"Cost: $0.0800",
		"Next: aigc-cli task task_1",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("footer missing %q:\n%s", want, text)
		}
	}
}

func TestSFXFooterShape(t *testing.T) {
	f := testutil.AbsPath(t, "/tmp/sfx.wav")
	text := withMCPResultFooter("Sound effect saved: "+f, []string{f}, "local: mmaudio", "", "", 0, "")
	for _, want := range []string{
		"Sound effect saved",
		"Saved:",
		f,
		"Model: local: mmaudio",
		"Next: aigc-cli preview " + f,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("SFX footer missing %q:\n%s", want, text)
		}
	}
}
