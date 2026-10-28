package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// DefaultSessionDirName is the directory name used under the config dir.
// Callers normally pass the value resolved from config/CLI instead.
const DefaultSessionDirName = "sessions"

var errReadOnly = errors.New("session is read-only")

// Target is the result of resolving a --session value.
type Target struct {
	Path   string
	ID     string
	Exists bool
}

// Session is an open JSONL transcript. A read-only Session (from Load) has no
// file handle and rejects append operations.
type Session struct {
	mu     sync.Mutex
	path   string
	f      *os.File
	header Header

	seq            int64
	name           string
	system         string
	messages       []seqMessage
	lastCompaction *CompactionRecord
	lastClear      *ClearRecord
}

type seqMessage struct {
	seq int64
	msg types.ChatMessage
}

// New creates a session file and writes its header. The file is created with
// owner-only permissions; parent directories use owner-only permissions too.
func New(path string, header Header) (*Session, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("session path is empty")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create session directory %s: %w", dir, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create session %s: %w", path, err)
	}

	header.Type = typeHeader
	header.Schema = Schema
	if header.CreatedAt.IsZero() {
		header.CreatedAt = time.Now().UTC()
	}
	if header.ID == "" {
		header.ID = NewID(time.Now())
	}
	if err := writeJSONLine(f, header); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("sync session %s: %w", path, err)
	}

	return &Session{path: path, f: f, header: header}, nil
}

// Open loads an existing session and opens it for appending.
func Open(path string) (*Session, error) {
	s, err := Load(path)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open session %s for append: %w", path, err)
	}
	s.f = f
	return s, nil
}

// Load reads a session without opening it for writes. It is used by --dry-run
// and by callers that only need the projected history.
func Load(path string) (*Session, error) {
	s := &Session{path: path}
	if err := s.read(); err != nil {
		return nil, err
	}
	return s, nil
}

// Path returns the session file path.
func (s *Session) Path() string { return s.path }

// ID returns the session id from the header.
func (s *Session) ID() string { return s.header.ID }

// Header returns a copy of the session header.
func (s *Session) Header() Header { return s.header }

// LastSeq returns the highest record sequence number written so far. It is used
// as the compaction/clear barrier.
func (s *Session) LastSeq() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.seq
}

// Name returns the latest stored session name, or "" when unnamed.
func (s *Session) Name() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.name
}

// AppendName records a session display name. The latest name wins on resume.
// Whitespace is collapsed and the name is capped so it fits the TUI header.
func (s *Session) AppendName(name string) error {
	name = normalizeSessionName(name)
	if name == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return errReadOnly
	}
	s.seq++
	rec := &SessionInfoRecord{Type: typeSessionInfo, Seq: s.seq, TS: time.Now().UTC(), Name: name}
	if err := writeJSONLine(s.f, rec); err != nil {
		return err
	}
	s.name = name
	return nil
}

// System returns the latest stored system prompt.
func (s *Session) System() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.system
}

// AppendSystem records the system prompt. The latest system record wins on
// resume.
func (s *Session) AppendSystem(content string) error {
	if content == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return errReadOnly
	}
	s.seq++
	rec := &SystemRecord{Type: typeSystem, Seq: s.seq, TS: time.Now().UTC(), Content: content}
	if err := writeJSONLine(s.f, rec); err != nil {
		return err
	}
	s.system = content
	return nil
}

// AppendMessage records one conversation message. Tool output is truncated to
// maxToolBytes (DefaultToolOutputMaxBytes when zero).
func (s *Session) AppendMessage(msg types.ChatMessage, name string, maxToolBytes int) error {
	content := msg.Content
	truncated := false
	originalBytes := 0
	if msg.Role == "tool" {
		if maxToolBytes <= 0 {
			maxToolBytes = DefaultToolOutputMaxBytes
		}
		var cut int
		content, truncated, cut = truncateToolContent(content, maxToolBytes)
		originalBytes = cut
	}

	rec := &MessageRecord{
		Type:          typeMessage,
		TS:            time.Now().UTC(),
		Role:          msg.Role,
		Content:       content,
		ToolCallID:    msg.ToolCallID,
		Name:          name,
		ToolCalls:     msg.ToolCalls,
		Annotations:   msg.Annotations,
		Images:        imageMetas(msg.Images),
		Truncated:     truncated,
		OriginalBytes: originalBytes,
	}
	if err := s.append(rec); err != nil {
		return err
	}

	s.mu.Lock()
	stored := msg
	stored.Content = content
	s.messages = append(s.messages, seqMessage{seq: rec.Seq, msg: stored})
	s.mu.Unlock()
	return nil
}

// AppendCompaction records a summary barrier. UpToSeq is the last message seq
// at the moment compaction runs, so everything before it is replaced by Summary
// on resume.
func (s *Session) AppendCompaction(summary string, tokensBefore, tokensAfter int, model string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return errReadOnly
	}
	upTo := s.seq
	s.seq++
	rec := &CompactionRecord{
		Type:         typeCompaction,
		Seq:          s.seq,
		TS:           time.Now().UTC(),
		UpToSeq:      upTo,
		Summary:      summary,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensAfter,
		Model:        model,
	}
	if err := writeJSONLine(s.f, rec); err != nil {
		return err
	}
	s.lastCompaction = rec
	s.dropMessagesUpTo(upTo)
	return nil
}

// AppendClear records a /clear barrier.
func (s *Session) AppendClear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return errReadOnly
	}
	rec := &ClearRecord{
		Type:    typeClear,
		Seq:     s.seq + 1,
		TS:      time.Now().UTC(),
		UpToSeq: s.seq,
	}
	s.seq = rec.Seq
	if err := writeJSONLine(s.f, rec); err != nil {
		return err
	}
	s.lastClear = rec
	s.dropMessagesUpTo(rec.UpToSeq)
	return nil
}

// History returns the conversation projected from the file: system prompt,
// latest compaction summary, and messages after the latest barrier.
func (s *Session) History() []types.ChatMessage {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]types.ChatMessage, 0, len(s.messages)+2)
	if s.system != "" {
		out = append(out, types.ChatMessage{Role: "system", Content: s.system})
	}

	barrierSeq := int64(0)
	summary := ""
	switch {
	case s.lastClear != nil && (s.lastCompaction == nil || s.lastClear.Seq > s.lastCompaction.Seq):
		barrierSeq = s.lastClear.UpToSeq
	case s.lastCompaction != nil:
		barrierSeq = s.lastCompaction.UpToSeq
		summary = s.lastCompaction.Summary
	}
	if summary != "" {
		out = append(out, types.ChatMessage{
			Role:    "system",
			Content: "Previous conversation summary:\n\n" + summary,
		})
	}

	for _, m := range s.messages {
		if m.seq > barrierSeq {
			out = append(out, m.msg)
		}
	}
	return out
}

// Close appends an end record, syncs and closes the file. It is a no-op for a
// read-only session.
func (s *Session) Close(reason string) error {
	s.mu.Lock()
	if s.f == nil {
		s.mu.Unlock()
		return nil
	}
	f := s.f
	s.seq++
	rec := &EndRecord{Type: typeEnd, Seq: s.seq, TS: time.Now().UTC(), Reason: reason}
	writeErr := writeJSONLine(f, rec)
	syncErr := f.Sync()
	closeErr := f.Close()
	s.f = nil
	s.mu.Unlock()

	switch {
	case writeErr != nil:
		return writeErr
	case syncErr != nil:
		return syncErr
	default:
		return closeErr
	}
}

// append assigns the next sequence number, writes the record and updates seq.
func (s *Session) append(rec seqSetter) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.f == nil {
		return errReadOnly
	}
	s.seq++
	rec.setSeq(s.seq)
	return writeJSONLine(s.f, rec)
}

// dropMessagesUpTo discards in-memory messages that a compaction or clear
// barrier has replaced. The records stay in the file; only the projection state
// is trimmed so a long session does not grow without bound.
func (s *Session) dropMessagesUpTo(seq int64) {
	if seq <= 0 {
		return
	}
	kept := s.messages[:0]
	for _, m := range s.messages {
		if m.seq > seq {
			kept = append(kept, m)
		}
	}
	s.messages = kept
}

func writeJSONLine(f *os.File, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal session record: %w", err)
	}
	data = append(data, '\n')
	for len(data) > 0 {
		n, err := f.Write(data)
		if err != nil {
			return fmt.Errorf("write session record: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("write session record: %w", io.ErrShortWrite)
		}
		data = data[n:]
	}
	return nil
}
