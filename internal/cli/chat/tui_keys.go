package chat

import (
	"io"
	"os"
	"strconv"
	"strings"
)

// Terminal key-reporting control sequences. modifyOtherKeys level 2 asks the
// terminal to report modified keys — including Alt+Enter, which the legacy
// encoding cannot express — while plain keystrokes keep their usual bytes.
// Level 2 also reports Ctrl+letter combinations in the extended form, so the
// reader below maps them back to the control bytes Bubble Tea expects.
const (
	enableModifiedKeys  = "\x1b[>4;2m"
	disableModifiedKeys = "\x1b[>4;0m"
)

// modifiedKeyInput wraps the TTY so Bubble Tea still recognises it as a
// terminal — it only enables raw mode (and restores it on exit) when the input
// exposes its file descriptor — while routing every read through the
// extended-key translator. Without the embedded *os.File the terminal would
// stay in cooked mode and typed keys would never reach the program.
type modifiedKeyInput struct {
	*os.File
	keys *modifiedKeyReader
}

func newModifiedKeyInput(f *os.File) *modifiedKeyInput {
	return &modifiedKeyInput{File: f, keys: newModifiedKeyReader(f)}
}

func (f *modifiedKeyInput) Read(p []byte) (int, error) { return f.keys.Read(p) }

// modifiedKeyReader normalises the extended key sequences that terminals emit
// once modifyOtherKeys (or the kitty keyboard protocol) is active:
//
//   - Shift+Enter / Alt+Enter / Ctrl+Enter become Ctrl+J, which the TUI treats
//     as "insert a line break" — Bubble Tea v1 cannot parse the raw sequences;
//   - Ctrl+letter and the other keys that level 2 re-encodes are mapped back to
//     their legacy bytes, so Ctrl+C, Ctrl+D, Ctrl+O, Tab, Backspace and Esc keep
//     working;
//   - anything it does not recognise is passed through untouched.
type modifiedKeyReader struct {
	r   io.Reader
	buf []byte // raw bytes that still need translating
	out []byte // translated bytes waiting to be handed to the caller
}

func newModifiedKeyReader(r io.Reader) *modifiedKeyReader {
	return &modifiedKeyReader{r: r}
}

func (r *modifiedKeyReader) Read(p []byte) (int, error) {
	for len(r.out) == 0 {
		chunk := make([]byte, 512)
		n, err := r.r.Read(chunk)
		if n > 0 {
			r.buf = append(r.buf, chunk[:n]...)
			r.translate(false)
		}
		if len(r.out) > 0 {
			break
		}
		if err != nil {
			// Flush a trailing incomplete sequence at EOF so no input is lost.
			if len(r.buf) > 0 {
				r.out = append(r.out, r.buf...)
				r.buf = nil
				break
			}
			return 0, err
		}
	}
	n := copy(p, r.out)
	r.out = r.out[n:]
	if len(r.out) == 0 {
		r.out = nil
	}
	return n, nil
}

// translate moves completed sequences from buf to out. An incomplete CSI
// sequence at the end of the buffer is kept until the next read; a lone ESC is
// passed through immediately so Escape still cancels a running turn without
// waiting for another key press.
func (r *modifiedKeyReader) translate(final bool) {
	i := 0
	for i < len(r.buf) {
		if r.buf[i] != 0x1b || i+1 >= len(r.buf) || r.buf[i+1] != '[' {
			r.out = append(r.out, r.buf[i])
			i++
			continue
		}
		rest := r.buf[i:]
		end := -1
		for j := 2; j < len(rest); j++ {
			if rest[j] >= 0x40 && rest[j] <= 0x7e {
				end = j
				break
			}
		}
		if end < 0 {
			if final {
				r.out = append(r.out, rest...)
				i = len(r.buf)
				break
			}
			break // incomplete: keep the tail for the next read
		}
		params := string(rest[2:end])
		finalByte := rest[end]
		if translated, ok := translateExtendedKey(params, finalByte); ok {
			r.out = append(r.out, translated...)
		} else {
			r.out = append(r.out, rest[:end+1]...)
		}
		i += end + 1
	}
	r.buf = r.buf[i:]
	if len(r.buf) == 0 {
		r.buf = nil
	}
}

// translateExtendedKey converts one extended key report into the legacy byte(s)
// Bubble Tea understands. It understands both encodings terminals use:
// "CSI 27 ; mods ; code ~" (xterm modifyOtherKeys) and "CSI code ; mods u"
// (kitty keyboard protocol).
func translateExtendedKey(params string, final byte) (string, bool) {
	parts := strings.Split(params, ";")
	var codeStr, modStr string
	switch final {
	case '~':
		if len(parts) != 3 || parts[0] != "27" {
			return "", false
		}
		codeStr, modStr = parts[2], parts[1]
	case 'u':
		if len(parts) != 2 {
			return "", false
		}
		codeStr, modStr = parts[0], parts[1]
	default:
		return "", false
	}

	// Kitty appends ":<event-type>" to the modifier parameter (1 = press,
	// 2 = repeat, 3 = release). Ignore release events so a key is not
	// inserted twice.
	modParts := strings.SplitN(modStr, ":", 2)
	if len(modParts) == 2 && modParts[1] != "1" && modParts[1] != "2" {
		return "", false
	}
	code, err := strconv.Atoi(codeStr)
	if err != nil || code <= 0 {
		return "", false
	}
	mod, err := strconv.Atoi(modParts[0])
	if err != nil || mod < 1 {
		return "", false
	}

	bits := mod - 1 // 1 = shift, 2 = alt, 4 = ctrl
	shift, alt, ctrl := bits&1 != 0, bits&2 != 0, bits&4 != 0

	// Enter with any modifier inserts a line break. A plain Enter is only
	// reported by terminals in the extended form (modifyOtherKeys level 2 on
	// some emulators); it keeps submitting the draft.
	if code == 13 {
		if bits == 0 {
			return "\r", true
		}
		return "\n", true
	}

	// Ctrl+letter (and the other control combinations) must keep arriving as
	// their control byte, otherwise Ctrl+C / Ctrl+D / Ctrl+O / Ctrl+J would
	// stop working once level 2 is enabled.
	if ctrl && !shift && !alt {
		if b, ok := controlByte(code); ok {
			return string(b), true
		}
	}

	// Unmodified or shift-only reporting: emit the character as typed.
	if bits == 0 {
		if s, ok := legacyKey(code); ok {
			return s, true
		}
	}
	if bits == 1 && isPrintable(code) {
		return string(rune(code)), true
	}
	// Alt+<key> keeps its legacy ESC prefix so it still reads as alt+key.
	if bits == 2 && isPrintable(code) {
		return "\x1b" + string(rune(code)), true
	}
	return "", false
}

// controlByte maps an ASCII key code to the control byte that Ctrl+<key>
// produces in the legacy encoding.
func controlByte(code int) (byte, bool) {
	switch {
	case code >= 'a' && code <= 'z':
		return byte(code-'a') + 1, true
	case code >= 'A' && code <= 'Z':
		return byte(code-'A') + 1, true
	case code == 32: // Ctrl+Space
		return 0x00, true
	case code == 63: // Ctrl+?
		return 0x7f, true
	case code >= 91 && code <= 95: // Ctrl+[ \ ] ^ _
		return byte(code - 64), true
	}
	return 0, false
}

// legacyKey returns the legacy encoding of an unmodified key that a terminal
// reported in the extended form.
func legacyKey(code int) (string, bool) {
	switch code {
	case 9:
		return "\t", true
	case 13:
		return "\r", true
	case 27:
		return "\x1b", true
	case 127:
		return "\x7f", true
	}
	if isPrintable(code) {
		return string(rune(code)), true
	}
	return "", false
}

func isPrintable(code int) bool {
	return code >= 0x20 && code != 0x7f && code <= 0x10ffff
}
