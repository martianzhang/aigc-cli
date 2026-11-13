package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/cliutil"
	"github.com/martianzhang/aigc-cli/internal/mcpcli"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// defaultMaxIterations caps how many LLM round-trips a single user message may
// trigger. Each round-trip can carry several tool calls, so this is a budget
// for the whole agentic turn, not a per-tool limit. It is deliberately large:
// an exploratory task routinely needs dozens of read/grep/exec steps, and the
// old cap of 10 cut turns off mid-task. defaults.chat.max_iterations overrides it.
const defaultMaxIterations = 100

// chat flag variables
var (
	chatSystem                 string
	chatMessages               []string
	chatTemperature            float64
	chatMaxTokens              int
	chatContextSize            int
	chatReserveTokens          int
	chatKeepRecentTokens       int
	chatNoAutoCompact          bool
	chatCompactModel           string
	chatCompactMaxTokens       int
	chatCompactToolResultChars int
	chatNoStream               bool
	chatJSONFlag               string
	chatInteractive            bool
	chatDryRun                 bool
	// chatNoTUI disables the interactive TUI even when stdin/stdout are
	// terminals. Agents running under a PTY use it to force single-turn mode.
	chatNoTUI bool
	// chatYes auto-approves confirm-gated tools in both TUI and non-interactive
	// mode.
	chatYes bool
	// chatSearchProvider overrides defaults.search.provider for the
	// web_search/web_fetch primitives when set on the CLI.
	chatSearchProvider string
	// chatInjectJSON is a --inject-json snippet merged additively into the body.
	chatInjectJSON string
	// chatInjectRaw is chatInjectJSON parsed once by buildChatRequest, so the
	// agent loop and TUI (which rebuild the request each turn) can re-apply it.
	chatInjectRaw json.RawMessage
	// chatSave enables JSONL session persistence for a new session.
	chatSave bool
	// chatListSessions lists saved sessions and exits without calling the API.
	chatListSessions bool
	// chatSessionLimit caps --list-sessions output (0 = all).
	chatSessionLimit int
	// chatSessionDir overrides the default session directory.
	chatSessionDir string
	// chatSession resumes an existing session by id or file path.
	chatSession string
	// chatName names a new session, or renames a resumed one.
	chatName string
	// chatSkillPaths are additional skill files/directories from --skill.
	chatSkillPaths []string
	// chatNoSkills disables automatic .agents/skills discovery.
	chatNoSkills bool
)

// chatCmd represents the `aigc-cli chat` command.
var chatCmd = &cobra.Command{
	Use:          "chat",
	Short:        "Chat with AI models (streaming by default)",
	SilenceUsage: true,
	Long: `Start a chat conversation with AI models via any OpenAI-compatible API.

Supports all major models: GPT, Claude, Gemini, DeepSeek, and more.
Streaming output is enabled by default.

Agentic Chat:
  Chat supports tool calling by default.

Sessions:
  Chat is not saved by default. --save writes a new JSONL session under the
  session directory; --session resumes an id or file path (and keeps saving).
  --name sets or changes the session name shown in the TUI header.
  --list-sessions prints the newest 20 saved sessions and exits; it needs no
  API key. --session-limit N changes the page size (0 = all).

Messages:
  --message is repeatable (one user turn per value). --prompt / -p is an
  alias, so either flag works; mixing them keeps command-line order. When a
  value names an existing file, its contents are used as the message.

Context Management:
  --context-size sets the input context window (0 = detect it from the model
  or configured provider). Near the window, older messages are summarized and
  a recent tail is kept verbatim. Use /context to inspect usage and /compact
  in interactive mode for manual compaction.

Modes:
  - Interactive multi-turn (default without --message):
      aigc-cli chat
  - Single-turn with --message:
      aigc-cli chat --message "Hello"

Examples:
  aigc-cli chat --message "Hello, who are you?"
  aigc-cli chat --prompt "Hello, who are you?"        # alias for --message
  aigc-cli chat --context-size 8192 --max-output 2048
  aigc-cli chat --save --name "review auth"
  aigc-cli chat --session 20261028T101500Z-ab12cd
  aigc-cli chat --session ./logs/review.jsonl
  aigc-cli chat --list-sessions
  aigc-cli chat --json '{"model":"gpt-5","messages":[{"role":"user","content":"Hi"}]}'`,
	RunE: runChat,
}

func runChat(cmd *cobra.Command, args []string) error {
	chatCfg := options.ChatDefaults()
	applyChatConfigDefaults(cmd, chatCfg)
	if chatListSessions {
		if err := validateListSessions(cmd); err != nil {
			return err
		}
		return runListSessions(cmd)
	}
	if err := validateSessionFlags(); err != nil {
		return err
	}

	// --json mode is always single-turn
	if chatJSONFlag != "" {
		req, err := buildChatRequest(cmd)
		if err != nil {
			return err
		}
		return sendChatRequest(cmd, req)
	}

	req, err := buildChatRequest(cmd)
	if err != nil {
		return err
	}
	if err := requireChatInput(req.Messages, chatInteractive, chatNoTUI); err != nil {
		return err
	}

	// --dry-run is single-turn: a preview must never open the interactive TUI
	// and must never create a session file.
	if chatDryRun {
		if chatSession != "" {
			sess, err := loadChatSessionReadOnly(chatCfg)
			if err != nil {
				return err
			}
			history, err := buildHistoryFromSession(sess, req.Messages, chatCfg, false)
			if err != nil {
				return err
			}
			req.Messages = history
		}
		return sendChatRequest(cmd, req)
	}

	// Interactive REPL (TUI) when no --message and not --json. Piped stdin is
	// non-interactive: it carries the message and flows through the agent loop
	// below, same as --message. --no-tui forces the same path for agents that
	// run under a PTY.
	if !chatNoTUI && (!messageFlagChanged(cmd) || chatInteractive) &&
		term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		return runChatTUI(cmd)
	}

	// Validate confirmation patterns before creating a session or calling the
	// API. The non-interactive agent loop uses this policy to fail closed.
	confirm, err := newConfirmPolicy(chatCfg)
	if err != nil {
		return err
	}

	sess, created, err := openChatSession(chatCfg)
	if err != nil {
		return err
	}
	if sess != nil {
		defer func() {
			closeSession(sess)
			printSessionPath(sess, created)
		}()
	}

	history, err := buildHistoryFromSession(sess, req.Messages, chatCfg, true)
	if err != nil {
		return err
	}

	// Non-interactive with --message(s): the agent loop below performs the
	// single send (with tools) and prints the answer once.
	maxIterations := defaultMaxIterations
	if chatCfg != nil && chatCfg.MaxIterations > 0 {
		maxIterations = chatCfg.MaxIterations
	}
	ctx := context.Background()
	mcpMgr, mcpErr := mcpcli.NewManager(ctx, options.MCPConfig())
	if mcpErr != nil {
		fmt.Fprintf(cliutil.Stderr(), "mcp: %v\n", mcpErr)
	}
	defer func() { _ = mcpMgr.Close() }()
	rt := newToolRuntime(chatCfg, mcpMgr)

	p := options.Shared.ResolveProvider(options.ProviderNameChat)
	if err := options.RequireBaseURL("chat", p); err != nil {
		return err
	}
	providerName := p.Name
	if providerName == "" {
		providerName = p.ProviderType.String()
	}
	model := options.Shared.Model
	if model == "" && chatCfg != nil {
		model = chatCfg.Model
	}
	settings := resolveCompactionSettings(cmd, chatCfg, sess, providerName, model)
	c := client.NewFromProvider(p)
	err = runAgentLoopWithSession(ctx, c, rt, &history, maxIterations, cmd, p, sess, sessionToolOutputMax(chatCfg), settings, confirm)
	if err != nil {
		return err
	}
	if options.Shared.Verbose && len(history) > len(req.Messages) {
		added := len(history) - len(req.Messages)
		fmt.Fprintf(cliutil.Stderr(), "Agent loop completed: %d additional messages accumulated (tool calls + responses)\n", added)
	}
	return nil
}

// requireChatInput rejects non-interactive invocations that have nothing to
// send. A non-TTY stdin with no --message must fail fast instead of entering
// the TUI, waiting for input, or sending an empty request.
func requireChatInput(messages []types.ChatMessage, interactive, noTUI bool) error {
	stdinTTY := chatStdinIsTerminal()
	stdoutTTY := term.IsTerminal(int(os.Stdout.Fd()))

	if interactive {
		if !stdinTTY || !stdoutTTY {
			return fmt.Errorf("--interactive requires a terminal\nHint: pass -p/--message for single-turn input, or pipe text on stdin")
		}
		return nil
	}
	if len(messages) > 0 {
		return nil
	}
	if noTUI || !stdinTTY || !stdoutTTY {
		return fmt.Errorf("no input: pass -p/--message, pipe text on stdin, or run 'aigc-cli chat --interactive'\nHint: aigc-cli chat -p \"hello\"")
	}
	return nil
}

// applyChatConfigDefaults fills flag variables from config when the flag
// was not set on the CLI. Config priority: CLI flags > config defaults.
func applyChatConfigDefaults(cmd *cobra.Command, cfg *types.ChatDefaults) {
	if cfg == nil {
		return
	}
	if !cmd.Flags().Changed("context-size") && cfg.ContextSize > 0 {
		chatContextSize = cfg.ContextSize
	}
	if !cmd.Flags().Changed("reserve-tokens") && cfg.ReserveTokens > 0 {
		chatReserveTokens = cfg.ReserveTokens
	}
	if !cmd.Flags().Changed("keep-recent-tokens") && cfg.KeepRecentTokens > 0 {
		chatKeepRecentTokens = cfg.KeepRecentTokens
	}
	if !cmd.Flags().Changed("no-auto-compact") && cfg.AutoCompact != nil && !*cfg.AutoCompact {
		chatNoAutoCompact = true
	}
	if cfg.CompactModel != "" {
		chatCompactModel = cfg.CompactModel
	}
	if cfg.CompactMaxTokens > 0 {
		chatCompactMaxTokens = cfg.CompactMaxTokens
	}
	if cfg.CompactToolResultChars > 0 {
		chatCompactToolResultChars = cfg.CompactToolResultChars
	}
	if !cmd.Flags().Changed("max-output") && cfg.MaxTokens > 0 {
		chatMaxTokens = cfg.MaxTokens
	}
	// Config defaults never conflict with an explicit session: --session
	// already implies saving. They also stay out of --json / --dry-run,
	// which do not write files.
	if !cmd.Flags().Changed("save") && cfg.Save && chatSession == "" && chatJSONFlag == "" && !chatDryRun {
		chatSave = true
	}
	if !cmd.Flags().Changed("session-dir") && cfg.SessionDir != "" {
		chatSessionDir = cfg.SessionDir
	}
}

func init() {
	registerFlags(chatCmd.Flags())
}

// BindFlags registers chat flags on another command. The root command uses it
// so that omitting the "chat" subcommand still accepts -p/--save/--session and
// the other chat options.
func BindFlags(cmd *cobra.Command) {
	registerFlags(cmd.Flags())
}

// registerFlags adds every chat-specific flag to f. It is shared by the chat
// subcommand and the root command; only one of them parses flags per
// invocation, and both bind to the same variables.
func registerFlags(f *pflag.FlagSet) {
	f.StringVarP(&chatSystem, "system", "s", "", "System prompt to set AI behavior")
	addMessageFlags(f)
	f.Float64VarP(&chatTemperature, "temperature", "t", 0, "Sampling temperature (0-2)")
	f.IntVar(&chatMaxTokens, "max-output", 0, "Maximum tokens in response")
	f.IntVar(&chatContextSize, "context-size", 0, "Input context window in tokens (0 = auto-detect from the model). Auto-compacts near the window while keeping recent messages.")
	f.IntVar(&chatReserveTokens, "reserve-tokens", 0, "Tokens reserved below the context window for output and tool schemas (0 = default 16384)")
	f.IntVar(&chatKeepRecentTokens, "keep-recent-tokens", 0, "Recent tokens kept verbatim after compaction (0 = default 20000)")
	f.BoolVar(&chatNoAutoCompact, "no-auto-compact", false, "Disable automatic context compaction (manual /compact still works)")
	f.BoolVar(&chatNoStream, "no-stream", false, "Disable streaming, wait for full response")
	f.StringVar(&chatJSONFlag, "json", "", "JSON file, string, or \"-\" for stdin")
	f.BoolVar(&chatInteractive, "interactive", false, "Enter interactive multi-turn chat mode")
	f.BoolVar(&chatNoTUI, "no-tui", false, "Never open the interactive TUI; require -p/--message or piped stdin")
	f.BoolVar(&chatDryRun, "dry-run", false, "Print the equivalent curl without calling the API")
	f.BoolVarP(&chatYes, "yes", "y", false, "Auto-approve confirm-gated tools in both TUI and non-interactive mode")
	f.StringVar(&chatSearchProvider, "search-provider", "", "Third-party vendor name for the web_search/web_fetch tools: duckduckgo/firecrawl/brave/doubao/deepseek/exa/wsa (overrides defaults.search.provider)")
	f.StringVar(&chatInjectJSON, "inject-json", "", "JSON object merged additively into the request body (arrays append, scalars overwrite); JSON string or file path")
	f.BoolVar(&chatSave, "save", false, "Save this chat as a JSONL session (default dir: ~/.config/aigc-cli/sessions)")
	f.BoolVar(&chatListSessions, "list-sessions", false, "List saved chat sessions and exit (newest 20 by default)")
	f.IntVar(&chatSessionLimit, "session-limit", sessionListLimit, "Max sessions shown by --list-sessions (0 = all)")
	f.StringVar(&chatSessionDir, "session-dir", "", "Session directory (default ~/.config/aigc-cli/sessions)")
	f.StringVar(&chatSession, "session", "", "Resume a session by id or JSONL file path (implies saving)")
	f.StringVar(&chatName, "name", "", "Session name (requires --save or --session; renames on resume)")
	f.StringArrayVar(&chatSkillPaths, "skill", nil, "Load a skill file or directory (repeatable)")
	f.BoolVar(&chatNoSkills, "no-skills", false, "Disable skills discovery and loading")
}

// Cmd returns the chat command.
func Cmd() *cobra.Command { return chatCmd }

// RunDefault runs chat as the root command's default action. Positional args
// are joined into one user message, so `aigc-cli hello` is equivalent to
// `aigc-cli -p hello`; with no args the TUI opens as usual.
func RunDefault(cmd *cobra.Command, args []string) error {
	if err := addPositionalMessage(args); err != nil {
		return err
	}
	return runChat(cmd, args)
}

// addPositionalMessage appends args as one user message. It rejects the
// ambiguous interactive combination instead of silently dropping the text.
func addPositionalMessage(args []string) error {
	if len(args) == 0 {
		return nil
	}
	if chatInteractive {
		return fmt.Errorf("--interactive cannot be combined with a positional message; use -p/--message instead")
	}
	chatMessages = append(chatMessages, strings.Join(args, " "))
	return nil
}
