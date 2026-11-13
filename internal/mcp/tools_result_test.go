package mcp

import (
	"strings"
	"testing"
)

func TestWithMCPResultFooter(t *testing.T) {
	text := withMCPResultFooter("已保存的图片: /tmp/a.png", []string{"/tmp/a.png"}, "gpt-image-2", "apimart", "task_1", 0.08, "aigc-cli task task_1")
	for _, want := range []string{
		"已保存的图片",
		"Saved:",
		"/tmp/a.png",
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
	text := withMCPResultFooter("Sound effect saved: /tmp/sfx.wav", []string{"/tmp/sfx.wav"}, "local: mmaudio", "", "", 0, "")
	for _, want := range []string{
		"Sound effect saved",
		"Saved:",
		"/tmp/sfx.wav",
		"Model: local: mmaudio",
		"Next: aigc-cli preview /tmp/sfx.wav",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("SFX footer missing %q:\n%s", want, text)
		}
	}
}
