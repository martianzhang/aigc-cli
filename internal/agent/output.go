package agent

import (
	"fmt"
	"sync"
	"unicode/utf8"
)

// MaxToolOutputBytes caps how much of a single tool result reaches the model
// and the in-memory conversation history. The session file has its own, much
// smaller cap (DefaultToolOutputMaxBytes); this one protects process memory and
// the next API request. One MiB is far larger than a useful model context while
// still preventing a fast producer from OOMing the CLI.
const MaxToolOutputBytes = 1 << 20

// shellOutputMarkerReserve leaves room for the truncation marker so the shell
// writer stops just short of MaxToolOutputBytes. The dispatch-level cap then
// sees an already-bounded string and does not truncate the marker itself.
const shellOutputMarkerReserve = 512

// CapToolOutput truncates an oversized tool result at a rune boundary and
// appends a marker explaining how much was omitted.
func CapToolOutput(s string) string {
	if len(s) <= MaxToolOutputBytes {
		return s
	}
	cut := MaxToolOutputBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + fmt.Sprintf(
		"\n\n[output truncated: showing %d of %d bytes; redirect to a file or use read/grep for the rest]",
		cut, len(s))
}

// cappedOutput is an io.Writer that keeps at most limit bytes while counting
// everything written. It lets a shell command stream into a fixed-size buffer
// instead of accumulating its full stdout/stderr in memory.
type cappedOutput struct {
	mu    sync.Mutex
	buf   []byte
	limit int
	total int64
}

func newCappedOutput(limit int) *cappedOutput {
	if limit <= 0 {
		limit = MaxToolOutputBytes
	}
	return &cappedOutput{limit: limit}
}

// Write never returns a short write: os/exec treats n < len(p) as
// io.ErrShortWrite and would abort the child's copy loop.
func (c *cappedOutput) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total += int64(len(p))
	if room := c.limit - len(c.buf); room > 0 {
		if len(p) <= room {
			c.buf = append(c.buf, p...)
		} else {
			c.buf = append(c.buf, p[:room]...)
		}
	}
	return len(p), nil
}

func (c *cappedOutput) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := string(c.buf)
	if c.total > int64(len(c.buf)) {
		s += fmt.Sprintf(
			"\n\n[output truncated: showing %d of %d bytes; redirect to a file or use read/grep for the rest]",
			len(c.buf), c.total)
	}
	return s
}
