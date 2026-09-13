package cmd

import (
	"context"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func defaultChatStyles() chatStyles {
	subtle := lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}
	userCol := lipgloss.AdaptiveColor{Light: "#2277DD", Dark: "#66AAFF"}
	asstCol := lipgloss.AdaptiveColor{Light: "#22AA66", Dark: "#55DD99"}
	toolCol := lipgloss.AdaptiveColor{Light: "#AA6622", Dark: "#DDAA55"}
	errCol := lipgloss.AdaptiveColor{Light: "#DD2222", Dark: "#FF5555"}

	return chatStyles{
		header: lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("#335577")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Bold(true).
			Width(80),
		messages: lipgloss.NewStyle().
			Padding(0, 1),
		inputBox: lipgloss.NewStyle().
			Border(lipgloss.Border{Left: "┃"}, false, false, false, true).
			BorderForeground(lipgloss.Color("#335577")).
			PaddingLeft(1),
		shellInputBox: lipgloss.NewStyle().
			Border(lipgloss.Border{Left: "┃"}, false, false, false, true).
			BorderForeground(lipgloss.Color("#DD8833")).
			PaddingLeft(1),
		statusBar: lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("#222222")).
			Foreground(lipgloss.Color("#CCCCCC")),
		userMsg: lipgloss.NewStyle().
			Foreground(userCol).
			Bold(true),
		asstMsg: lipgloss.NewStyle().
			Foreground(asstCol),
		sysMsg: lipgloss.NewStyle().
			Foreground(subtle).
			Italic(true),
		toolMsg: lipgloss.NewStyle().
			Foreground(toolCol),
		errMsg: lipgloss.NewStyle().
			Foreground(errCol),
	}
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

func newChatModel(c *client.Client, tools []types.ToolDefinition, maxIt int, mdl, sys string,
	cobraCmd *cobra.Command, verb bool, temp float64, maxTok, ctxSize int) chatModel {

	ctx, cancel := context.WithCancel(context.Background())

	ti := textarea.New()
	ti.Focus()
	ti.Placeholder = "Enter to send · Alt+Enter newline · PgUp/PgDn scroll"
	ti.Prompt = ""
	ti.ShowLineNumbers = false
	ti.CharLimit = 0
	// 999 effectively removes the width cap — real width is set via SetWidth on resize
	ti.MaxWidth = 999
	ti.MaxHeight = 5
	// Minimal style — no cursor line highlight
	ti.FocusedStyle.CursorLine = lipgloss.NewStyle()

	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#335577", Dark: "#66AAFF"})
	s.Spinner = spinner.Dot

	// Build completion list from tool definitions
	completions := []string{"/exit", "/quit", "/q", "/clear", "/reset", "/new", "/help", "/?", "/tools", "/copy"}
	for _, t := range tools {
		completions = append(completions, "/"+t.Function.Name)
	}

	// Pre-fill with welcome message so renderMessages always renders messages
	welcomeMsg := buildWelcomeBanner()

	return chatModel{
		state:       tuiIdle,
		client:      c,
		agentTools:  tools,
		maxIters:    maxIt,
		model:       mdl,
		cmd:         cobraCmd,
		verbose:     verb,
		temperature: temp,
		maxTokens:   maxTok,
		contextSize: ctxSize,
		input:       ti,
		spinner:     s,
		messages:    []message{{role: "system", content: welcomeMsg}},
		completions: completions,
		cycleIdx:    -1,
		histIdx:     -1,
		streamBuf:   &strings.Builder{},
		mu:          &sync.Mutex{},
		system:      sys, // store original for /clear; date hint in history separately
		ctx:         ctx,
		cancel:      cancel,
		styles:      defaultChatStyles(),
	}
}

// ---------------------------------------------------------------------------
// Init — Bubble Tea lifecycle
// ---------------------------------------------------------------------------
