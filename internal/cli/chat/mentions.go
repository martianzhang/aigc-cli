package chat

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	// maxInlineBytes caps how much of a text file an @-mention inlines, so a
	// huge file cannot blow up the request or the model's context window.
	maxInlineBytes = 1 << 20 // 1 MiB
	// maxImageBytes caps a single attached image (matches OpenCode's 20 MiB).
	maxImageBytes = 20 << 20 // 20 MiB
)

// expandMentions rewrites @path references in a user message:
//
//   - Text files are inlined in place as <file name="path">…</file>.
//   - Images are returned as ChatImage attachments and replaced by a short
//     marker so the model still sees which file was attached.
//   - Tokens that do not resolve to a readable regular file are left untouched,
//     so "@handle" and "@scope/pkg" are not mistaken for paths.
//
// Security: every read goes through agent.ReadFileRaw, so defaults.chat.read_deny
// is enforced exactly as it is for the read tool. Binary and oversized files are
// refused (and reported as warnings) rather than silently inlined.
func expandMentions(text string, deny []string) (string, []types.ChatImage, []string) {
	var (
		out      strings.Builder
		images   []types.ChatImage
		warnings []string
	)
	rest := text
	for {
		at := mentionStart(rest)
		if at < 0 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:at])
		token, tail := splitMention(rest[at:])
		inline, imgs, warns := resolveMention(strings.TrimPrefix(token, "@"), deny)
		out.WriteString(inline)
		images = append(images, imgs...)
		warnings = append(warnings, warns...)
		rest = tail
	}
	return out.String(), images, warnings
}

// mentionStart returns the index of the first '@' that begins a token (start of
// string, or preceded by whitespace), or -1 when there is none.
func mentionStart(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] != '@' {
			continue
		}
		if i == 0 || isMentionSpace(s[i-1]) {
			return i
		}
	}
	return -1
}

// splitMention splits an @token off the front of s, returning the token and the
// remaining text (the delimiter is kept on the remainder).
func splitMention(s string) (token, rest string) {
	for i := 0; i < len(s); i++ {
		if isMentionSpace(s[i]) {
			return s[:i], s[i:]
		}
	}
	return s, ""
}

func isMentionSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// resolveMention reads one @path reference, returning the text to substitute,
// any image attachments, and user-facing warnings.
func resolveMention(path string, deny []string) (string, []types.ChatImage, []string) {
	token := "@" + path
	if path == "" {
		return token, nil, nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		// Not an attachable file: leave the token as literal text.
		return token, nil, nil
	}
	data, err := agent.ReadFileRaw(path, deny)
	if err != nil {
		return token, nil, []string{fmt.Sprintf("not attached %s: %v", path, err)}
	}
	if mime, ok := imageMimeFor(data, path); ok {
		if len(data) > maxImageBytes {
			return token, nil, []string{fmt.Sprintf(
				"not attached %s: image exceeds %d MiB", path, maxImageBytes>>20)}
		}
		img := types.ChatImage{Name: filepath.Base(path), MIMEType: mime, Data: data}
		return fmt.Sprintf("[attached image: %s]", path), []types.ChatImage{img}, nil
	}
	if agent.LooksBinary(data) {
		return token, nil, []string{fmt.Sprintf("not inlined %s: binary file", path)}
	}
	if len(data) > maxInlineBytes {
		return token, nil, []string{fmt.Sprintf(
			"not inlined %s: %d bytes exceeds the %d KiB inline limit",
			path, len(data), maxInlineBytes>>10)}
	}
	return fmt.Sprintf("<file name=%q>\n%s\n</file>", path, string(data)), nil, nil
}

// imageMimeFor reports whether data/path denotes an attachable image and, if so,
// its media type. Content sniffing wins over the extension; SVG is deliberately
// treated as text (matching OpenCode).
func imageMimeFor(data []byte, path string) (string, bool) {
	if len(data) > 0 {
		switch ct := http.DetectContentType(data); ct {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
			return ct, true
		}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".gif":
		return "image/gif", true
	case ".webp":
		return "image/webp", true
	}
	return "", false
}
