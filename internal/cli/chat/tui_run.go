package chat

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/mcpcli"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// runChatTUI initialises and runs the Bubble Tea TUI chat program.
func runChatTUI(cmd *cobra.Command) error {
	// Load chat config for agent loop settings
	chatCfg := options.ChatDefaults()
	if options.Shared.Model == "" && chatCfg != nil && chatCfg.Model != "" {
		options.Shared.Model = chatCfg.Model
	}

	// Compile the tool-confirmation policy up front so an invalid regex fails
	// before the TUI takes over the screen.
	confirm, err := newConfirmPolicy(chatCfg)
	if err != nil {
		return err
	}

	maxIterations := 10
	if chatCfg != nil && chatCfg.MaxIterations > 0 {
		maxIterations = chatCfg.MaxIterations
	}

	mcpMgr, mcpErr := mcpcli.NewManager(context.Background(), options.MCPConfig())
	if mcpErr != nil {
		fmt.Fprintf(options.Stderr(), "mcp: %v\n", mcpErr)
	}
	defer mcpMgr.Close()

	rt := newToolRuntime(chatCfg, mcpMgr)
	agentTools := rt.RequestTools()
	c := options.NewClient("chat")

	sess, created, err := openChatSession(chatCfg)
	if err != nil {
		return err
	}
	if sess != nil {
		defer func() {
			closeSession(sess)
			printSessionPath(sess, created)
		}()
		if sess.System() != "" && chatSystem != "" {
			return fmt.Errorf("--session cannot be combined with --system; the system prompt is stored in the session")
		}
		if !cmd.Flags().Changed("context-size") && chatContextSize == 0 {
			chatContextSize = sess.Header().ContextSize
		}
	}

	// Initialize history with the stored system prompt, or build the current
	// one for a new session. The system prompt is persisted with the session,
	// so resuming uses the prompt the conversation actually ran with.
	history := []types.ChatMessage{}
	sysContent := ""
	resumed := sess != nil && !created
	if sess != nil && sess.System() != "" {
		sysContent = sess.System()
		history = sess.History()
	} else {
		sysContent = buildTUISystemPrompt()
		if sess != nil {
			if err := sess.AppendSystem(sysContent); err != nil {
				return err
			}
			history = sess.History()
		} else {
			history = append(history, types.ChatMessage{Role: "system", Content: sysContent})
		}
	}

	model := options.Shared.Model
	if model == "" && chatCfg != nil {
		model = chatCfg.Model
	}

	// Resolve the effective chat provider for the status bar (named key, else
	// the detected provider type).
	providerName := ""
	if p := options.Shared.ResolveProvider(options.ProviderNameChat); p != nil {
		providerName = p.Name
		if providerName == "" {
			if s := p.ProviderType.String(); s != "" && s != "unknown" {
				providerName = s
			}
		}
	}

	// Build the TUI model
	tuiModel := newChatModel(c, agentTools, maxIterations, model, sysContent, cmd, options.Shared.Verbose,
		chatTemperature, chatMaxTokens, chatContextSize)
	tuiModel.history = history
	tuiModel.rt = rt
	tuiModel.provider = providerName
	tuiModel.confirm = confirm
	tuiModel.sess = sess
	tuiModel.toolOutputMax = sessionToolOutputMax(chatCfg)
	if resumed && sess != nil {
		tuiModel.messages = append(tuiModel.messages, resumedNotice(sess, len(history)))
		tuiModel.messages = append(tuiModel.messages, resumedChatMessages(history)...)
	}

	// Create the Bubble Tea program with alt screen. Mouse capture is enabled so
	// the wheel scrolls the chat viewport; hold the terminal's mouse-reporting
	// bypass key to select text natively (Shift on most terminals, Option/⌥ on
	// iTerm2).
	//
	// Modified Enter (Alt/Shift/Ctrl) must insert a newline rather than submit.
	// Ask the terminal to report modified keys and translate the sequences it
	// sends back to what Bubble Tea understands — v1 has no parser for them.
	fmt.Fprint(os.Stdout, enableModifiedKeys)
	defer fmt.Fprint(os.Stdout, disableModifiedKeys)

	p := tea.NewProgram(
		tuiModel,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithInput(newModifiedKeyInput(os.Stdin)),
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
	_, err = p.Run()
	setProgram(nil)
	return err
}
