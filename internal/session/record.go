// Package session persists chat conversations as append-only JSONL files.
package session

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// Schema identifies the JSONL layout written by this package.
const Schema = "aigc.session.v1"

// DefaultToolOutputMaxBytes caps how much of a single tool result is stored.
// Session transcripts are meant to survive long agent runs; keeping every byte
// of bash/read output would make files grow without bound.
const DefaultToolOutputMaxBytes = 32 * 1024

// Record type names.
const (
	typeHeader      = "header"
	typeSystem      = "system"
	typeMessage     = "message"
	typeCompaction  = "compaction"
	typeClear       = "clear"
	typeSessionInfo = "session_info"
	typeEnd         = "end"
)

// Header is the first line of a session file. It identifies the session and
// records the environment that created it.
type Header struct {
	Type        string    `json:"type"`
	Schema      string    `json:"schema"`
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	Cwd         string    `json:"cwd,omitempty"`
	Provider    string    `json:"provider,omitempty"`
	Model       string    `json:"model,omitempty"`
	ContextSize int       `json:"context_size,omitempty"`
	App         string    `json:"app,omitempty"`
}

// SystemRecord stores the system prompt used by the conversation, including
// project context (AGENTS.md). The latest system record wins on resume.
type SystemRecord struct {
	Type    string    `json:"type"`
	Seq     int64     `json:"seq"`
	TS      time.Time `json:"ts"`
	Content string    `json:"content"`
}

// ImageMeta is the metadata kept for an attached image. Image bytes are not
// stored: base64 data would turn one line into megabytes and break line-based
// tooling.
type ImageMeta struct {
	Name     string `json:"name,omitempty"`
	MIMEType string `json:"mime,omitempty"`
	Bytes    int    `json:"bytes,omitempty"`
}

// MessageRecord is one conversation message. The shape mirrors the OpenAI wire
// format plus optional metadata that does not participate in requests.
type MessageRecord struct {
	Type          string                 `json:"type"`
	Seq           int64                  `json:"seq"`
	TS            time.Time              `json:"ts"`
	Role          string                 `json:"role"`
	Content       string                 `json:"content,omitempty"`
	ToolCallID    string                 `json:"tool_call_id,omitempty"`
	Name          string                 `json:"name,omitempty"`
	ToolCalls     []types.ToolCall       `json:"tool_calls,omitempty"`
	Annotations   []types.ChatAnnotation `json:"annotations,omitempty"`
	Images        []ImageMeta            `json:"images,omitempty"`
	Truncated     bool                   `json:"truncated,omitempty"`
	OriginalBytes int                    `json:"original_bytes,omitempty"`
}

// CompactionRecord records a summarized history checkpoint. Messages up to and
// including UpToSeq remain in the file but are replaced by Summary when the
// conversation is resumed.
type CompactionRecord struct {
	Type         string    `json:"type"`
	Seq          int64     `json:"seq"`
	TS           time.Time `json:"ts"`
	UpToSeq      int64     `json:"up_to_seq"`
	Summary      string    `json:"summary"`
	TokensBefore int       `json:"tokens_before,omitempty"`
	TokensAfter  int       `json:"tokens_after,omitempty"`
	Model        string    `json:"model,omitempty"`
}

// SessionInfoRecord stores mutable session metadata. The latest name wins.
type SessionInfoRecord struct {
	Type string    `json:"type"`
	Seq  int64     `json:"seq"`
	TS   time.Time `json:"ts"`
	Name string    `json:"name"`
}

// ClearRecord is a barrier created by /clear. Messages up to UpToSeq are not
// part of the resumed history.
type ClearRecord struct {
	Type    string    `json:"type"`
	Seq     int64     `json:"seq"`
	TS      time.Time `json:"ts"`
	UpToSeq int64     `json:"up_to_seq"`
}

// EndRecord marks a clean end of the session writer.
type EndRecord struct {
	Type   string    `json:"type"`
	Seq    int64     `json:"seq"`
	TS     time.Time `json:"ts"`
	Reason string    `json:"reason,omitempty"`
}

// seqSetter is implemented by every record that carries a sequence number.
type seqSetter interface {
	setSeq(int64)
}

func (r *SystemRecord) setSeq(seq int64)      { r.Seq = seq }
func (r *MessageRecord) setSeq(seq int64)     { r.Seq = seq }
func (r *CompactionRecord) setSeq(seq int64)  { r.Seq = seq }
func (r *ClearRecord) setSeq(seq int64)       { r.Seq = seq }
func (r *SessionInfoRecord) setSeq(seq int64) { r.Seq = seq }
func (r *EndRecord) setSeq(seq int64)         { r.Seq = seq }

func (r *MessageRecord) toChatMessage() types.ChatMessage {
	return types.ChatMessage{
		Role:        r.Role,
		Content:     r.Content,
		ToolCallID:  r.ToolCallID,
		ToolCalls:   r.ToolCalls,
		Annotations: r.Annotations,
	}
}

// normalizeSessionName collapses whitespace and caps the display name so it
// cannot break the TUI header.
func normalizeSessionName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	runes := []rune(name)
	if len(runes) > 80 {
		name = string(runes[:80])
	}
	return name
}

func imageMetas(images []types.ChatImage) []ImageMeta {
	if len(images) == 0 {
		return nil
	}
	out := make([]ImageMeta, 0, len(images))
	for _, img := range images {
		out = append(out, ImageMeta{Name: img.Name, MIMEType: img.MIMEType, Bytes: len(img.Data)})
	}
	return out
}

// truncateToolContent cuts s to at most maxBytes without splitting a UTF-8
// rune and appends a marker that tells the model the output was shortened.
func truncateToolContent(s string, maxBytes int) (string, bool, int) {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s, false, 0
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return fmt.Sprintf("%s\n\n[truncated: showing %d of %d bytes]", s[:cut], cut, len(s)),
		true, len(s)
}
