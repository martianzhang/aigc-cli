package chat

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/session"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// validateSessionFlags rejects combinations that cannot be represented by the
// session model.
func validateSessionFlags() error {
	if chatSave && chatSession != "" {
		return fmt.Errorf("--save and --session cannot be used together")
	}
	if chatJSONFlag != "" && (chatSave || chatSession != "") {
		return fmt.Errorf("--json cannot be combined with --save or --session")
	}
	if chatDryRun && chatSave {
		return fmt.Errorf("--dry-run cannot be combined with --save")
	}
	if chatName != "" && !chatSave && chatSession == "" {
		return fmt.Errorf("--name requires --save or --session")
	}
	return nil
}

func sessionDir(cfg *types.ChatDefaults) string {
	if chatSessionDir != "" {
		return expandHome(chatSessionDir)
	}
	if cfg != nil && cfg.SessionDir != "" {
		return expandHome(cfg.SessionDir)
	}
	return filepath.Join(options.ConfigDir(), session.DefaultSessionDirName)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	return path
}

func sessionToolOutputMax(cfg *types.ChatDefaults) int {
	if cfg != nil && cfg.SessionToolOutputMaxBytes > 0 {
		return cfg.SessionToolOutputMaxBytes
	}
	return session.DefaultToolOutputMaxBytes
}

// openChatSession creates or opens the session selected by --save / --session.
// The bool result reports whether a new file was created.
func openChatSession(cfg *types.ChatDefaults) (*session.Session, bool, error) {
	if !chatSave && chatSession == "" {
		return nil, false, nil
	}

	dir := sessionDir(cfg)
	header := currentSessionHeader()

	var (
		sess    *session.Session
		created bool
		err     error
	)
	if chatSession != "" {
		target, resolveErr := session.Resolve(chatSession, dir)
		if resolveErr != nil {
			return nil, false, resolveErr
		}
		if target.Exists {
			sess, err = session.Open(target.Path)
		} else {
			header.ID = target.ID
			sess, err = session.New(target.Path, header)
			created = true
		}
	} else {
		header.ID = session.NewID(time.Now())
		path := filepath.Join(dir, header.ID+".jsonl")
		sess, err = session.New(path, header)
		created = true
	}
	if err != nil {
		return nil, false, err
	}
	if chatName != "" {
		if err := sess.AppendName(chatName); err != nil {
			sess.Close("error")
			return nil, false, err
		}
	}
	return sess, created, nil
}

// loadChatSessionReadOnly opens a session without append access. --dry-run uses
// it to build a request without touching the file.
func loadChatSessionReadOnly(cfg *types.ChatDefaults) (*session.Session, error) {
	if chatSession == "" {
		return nil, nil
	}
	target, err := session.Resolve(chatSession, sessionDir(cfg))
	if err != nil {
		return nil, err
	}
	if !target.Exists {
		return nil, fmt.Errorf("session %q not found; --dry-run does not create sessions", chatSession)
	}
	return session.Load(target.Path)
}

func currentSessionHeader() session.Header {
	cwd, _ := os.Getwd()

	providerName := ""
	if p := options.Shared.ResolveProvider(options.ProviderNameChat); p != nil {
		providerName = p.Name
		if providerName == "" {
			if s := p.ProviderType.String(); s != "" && s != "unknown" {
				providerName = s
			}
		}
	}

	model := options.Shared.Model
	if model == "" {
		if cfg := options.ChatDefaults(); cfg != nil {
			model = cfg.Model
		}
	}

	return session.Header{
		Cwd:         cwd,
		Provider:    providerName,
		Model:       model,
		ContextSize: chatContextSize,
		App:         "aigc-cli",
	}
}

// buildHistoryFromSession combines the persisted projection with the new user
// messages. When persist is true, the system prompt and new user messages are
// appended to the file first.
func buildHistoryFromSession(sess *session.Session, newMessages []types.ChatMessage, cfg *types.ChatDefaults, persist bool) ([]types.ChatMessage, error) {
	system, messages := splitLeadingSystem(newMessages)

	if sess == nil {
		if system == "" {
			system = loadAgentsContext()
		}
		return withSystem(system, messages), nil
	}

	storedSystem := sess.System()
	if system != "" && storedSystem != "" {
		return nil, fmt.Errorf("--system cannot be combined with --session; the system prompt is stored in the session")
	}

	if persist {
		if storedSystem == "" && system == "" {
			system = loadAgentsContext()
		}
		if storedSystem == "" && system != "" {
			if err := sess.AppendSystem(system); err != nil {
				return nil, err
			}
		}
		for _, msg := range messages {
			if err := sess.AppendMessage(msg, "", sessionToolOutputMax(cfg)); err != nil {
				return nil, err
			}
		}
		return sess.History(), nil
	}

	history := sess.History()
	if storedSystem == "" && system == "" {
		system = loadAgentsContext()
	}
	if storedSystem == "" && system != "" {
		history = withSystem(system, history)
	}
	return append(history, messages...), nil
}

func splitLeadingSystem(messages []types.ChatMessage) (string, []types.ChatMessage) {
	if len(messages) > 0 && messages[0].Role == "system" {
		return messages[0].Content, messages[1:]
	}
	return "", messages
}

func withSystem(system string, messages []types.ChatMessage) []types.ChatMessage {
	if system == "" {
		return messages
	}
	out := make([]types.ChatMessage, 0, len(messages)+1)
	out = append(out, types.ChatMessage{Role: "system", Content: system})
	return append(out, messages...)
}

func closeSession(sess *session.Session) {
	if sess == nil {
		return
	}
	if err := sess.Close("exit"); err != nil {
		fmt.Fprintf(options.Stderr(), "session: %v\n", err)
	}
}

func printSessionPath(sess *session.Session, created bool) {
	if sess == nil || !created {
		return
	}
	out := options.Stderr()
	fmt.Fprintf(out, "Session: %s\n", sess.Path())
	if name := sess.Name(); name != "" {
		fmt.Fprintf(out, "Name:    %s\n", name)
	}
	fmt.Fprintf(out, "Resume:  %s\n", resumeCommand(sess, options.ChatDefaults()))
}

// resumeCommand renders the shortest --session command that reopens sess.
// Sessions under the active session directory use the id; an explicit path is
// used as-is. A non-default directory is repeated so the command works in a
// fresh shell.
func resumeCommand(sess *session.Session, cfg *types.ChatDefaults) string {
	dir := filepath.Clean(sessionDir(cfg))
	path := filepath.Clean(sess.Path())
	defaultDir := filepath.Clean(filepath.Join(options.ConfigDir(), session.DefaultSessionDirName))

	if filepath.Dir(path) == dir {
		cmd := "aigc-cli chat --session " + sess.ID()
		if dir != defaultDir {
			cmd += " --session-dir " + shellQuote(dir)
		}
		return cmd
	}
	return "aigc-cli chat --session " + shellQuote(path)
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
