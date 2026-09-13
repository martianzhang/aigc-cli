package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// runChatTUI initialises and runs the Bubble Tea TUI chat program.
func runChatTUI(cmd *cobra.Command) error {
	// Load chat config for agent loop settings
	chatCfg := chatDefaults()
	if shared.Model == "" && chatCfg != nil && chatCfg.Model != "" {
		shared.Model = chatCfg.Model
	}

	maxIterations := 10
	if chatCfg != nil && chatCfg.MaxIterations > 0 {
		maxIterations = chatCfg.MaxIterations
	}

	agentTools := buildAgentTools(chatCfg)
	c := newCmdClient("chat")

	// Initialize history with system prompt + current date context
	history := []types.ChatMessage{}
	sysContent := fmt.Sprintf("今天是 %s。你只需要在用户明确询问日期时才回答日期，其他时候不要主动提及。", time.Now().Format("2006年1月2日"))
	if chatSystem != "" {
		sysContent += "\n" + chatSystem
	}
	history = append(history, types.ChatMessage{Role: "system", Content: sysContent})

	model := shared.Model
	if model == "" && chatCfg != nil {
		model = chatCfg.Model
	}

	// Build the TUI model
	tuiModel := newChatModel(c, agentTools, maxIterations, model, chatSystem, cmd, shared.Verbose,
		chatTemperature, chatMaxTokens, chatContextSize)
	tuiModel.history = history

	// Create the Bubble Tea program with alt screen
	// (No mouse capture — lets native text selection work)
	p := tea.NewProgram(
		&tuiModel,
		tea.WithAltScreen(),
	)

	// Listen for SIGTERM to cleanly exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	go func() {
		<-sigCh
		p.Quit()
	}()

	// Store program reference for goroutines
	setProgram(p)

	// Run
	_, err := p.Run()
	setProgram(nil)
	return err
}
