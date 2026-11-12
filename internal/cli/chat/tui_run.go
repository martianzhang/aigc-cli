package chat

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/cliutil"
	"github.com/martianzhang/aigc-cli/internal/mcpcli"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
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

	maxIterations := defaultMaxIterations
	if chatCfg != nil && chatCfg.MaxIterations > 0 {
		maxIterations = chatCfg.MaxIterations
	}

	mcpMgr, mcpErr := mcpcli.NewManager(context.Background(), options.MCPConfig())
	if mcpErr != nil {
		fmt.Fprintf(cliutil.Stderr(), "mcp: %v\n", mcpErr)
	}
	defer func() { _ = mcpMgr.Close() }()

	rt := newToolRuntime(chatCfg, mcpMgr)
	agentTools := rt.RequestTools()

	sess, created, err := openChatSession(chatCfg)
	if err != nil {
		return err
	}
	// The slot, not the startup pointer, is what the exit handler closes: after
	// /session switches the active session is a different file.
	slot := &sessionSlot{sess: sess}
	defer func() {
		closeSession(slot.sess)
		printSessionPath(slot.sess, created || slot.switched)
	}()
	if sess != nil {
		if sess.System() != "" && chatSystem != "" {
			return fmt.Errorf("--session cannot be combined with --system; the system prompt is stored in the session")
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
		} else if sysContent != "" {
			history = append(history, types.ChatMessage{Role: "system", Content: sysContent})
		}
	}

	// A resumed session remembers the last /provider and /model selection,
	// unless --provider / --model or defaults.chat.* already chose one.
	chatProvider := resumeProvider(chatCfg, sess)
	providerName := providerDisplayName(chatProvider)
	model := resumeModel(sess, chatProvider)
	if err := options.RequireBaseURL(options.ProviderNameChat, chatProvider); err != nil {
		return err
	}
	c := client.NewFromProvider(chatProvider)

	settings := resolveCompactionSettings(cmd, chatCfg, sess, providerName, model)

	// Build the TUI model
	tuiModel := newChatModel(c, agentTools, maxIterations, model, sysContent, cmd, options.Shared.Verbose,
		chatTemperature, chatMaxTokens, settings.Window)
	tuiModel.history = history
	tuiModel.rt = rt
	tuiModel.provider = providerName
	tuiModel.confirm = confirm
	tuiModel.sess = sess
	tuiModel.sessSlot = slot
	tuiModel.sessionDirPath = sessionDir(chatCfg)
	tuiModel.toolOutputMax = sessionToolOutputMax(chatCfg)
	tuiModel.compactSettings = settings
	tuiModel.toolsTokens = agent.EstimateToolDefinitions(agentTools) + agent.EstimateRequestExtras(chatInjectRaw)
	tuiModel.contextTokens = agent.EstimateContext(history, nil, tuiModel.toolsTokens)
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
	ui.Fprint(ui.Out(), enableModifiedKeys)
	defer ui.Fprint(ui.Out(), disableModifiedKeys)

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

	// A stray write to the process stderr would land in the middle of the alt
	// screen and can leave stale rows (for example, a model-switch warning).
	// Tool/agent logs already redirect stderr to logWriter explicitly.
	restoreStderr := cliutil.SetStderr(io.Discard)
	defer restoreStderr()

	// Store program reference for goroutines
	setProgram(p)

	// Run
	_, err = p.Run()
	setProgram(nil)
	return err
}
