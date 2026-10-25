package chat

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

// chunkReader hands out one pre-set chunk per Read call, simulating a terminal
// whose escape sequence is split across reads.
type chunkReader struct {
	chunks []string
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(c.chunks) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.chunks[0])
	c.chunks = c.chunks[1:]
	return n, nil
}

func readAll(t *testing.T, in string) string {
	t.Helper()
	var out bytes.Buffer
	if _, err := io.Copy(&out, newModifiedKeyReader(strings.NewReader(in))); err != nil {
		t.Fatalf("copy: %v", err)
	}
	return out.String()
}

func TestModifiedKeyReader(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// Modified Enter → line break (Ctrl+J).
		{"shift+enter (modifyOtherKeys)", "\x1b[27;2;13~", "\n"},
		{"alt+enter (modifyOtherKeys)", "\x1b[27;3;13~", "\n"},
		{"shift+alt+enter", "\x1b[27;4;13~", "\n"},
		{"ctrl+enter", "\x1b[27;5;13~", "\n"},
		{"shift+enter (kitty)", "\x1b[13;2u", "\n"},
		{"kitty with event type", "\x1b[13;2:1u", "\n"},
		{"kitty release ignored", "\x1b[13;2:3u", "\x1b[13;2:3u"},
		{"ctrl+j via kitty", "\x1b[106;5u", "\n"},

		// Keys that must keep working once level 2 re-encodes them.
		{"ctrl+c", "\x1b[27;5;99~", "\x03"},
		{"ctrl+d", "\x1b[27;5;100~", "\x04"},
		{"ctrl+j", "\x1b[27;5;106~", "\n"},
		{"ctrl+o", "\x1b[27;5;111~", "\x0f"},
		{"ctrl+m (enter)", "\x1b[27;5;109~", "\r"},
		{"ctrl+[ (escape)", "\x1b[27;5;91~", "\x1b"},
		{"ctrl+i (tab)", "\x1b[27;5;105~", "\t"},
		{"plain enter reported", "\x1b[27;1;13~", "\r"},
		{"plain tab reported", "\x1b[27;1;9~", "\t"},
		{"plain escape reported", "\x1b[27;1;27~", "\x1b"},
		{"plain backspace reported", "\x1b[27;1;127~", "\x7f"},
		{"shift+a", "\x1b[27;2;65~", "A"},
		{"alt+a keeps escape prefix", "\x1b[27;3;97~", "\x1ba"},
		{"ctrl+shift+c is not an interrupt", "\x1b[27;6;99~", "\x1b[27;6;99~"},

		// Everything else passes through untouched.
		{"plain enter", "\r", "\r"},
		{"plain ctrl+j", "\n", "\n"},
		{"legacy alt+enter", "\x1b\r", "\x1b\r"},
		{"lone escape", "\x1b", "\x1b"},
		{"other sequence untouched", "a\x1b[1;5Ab", "a\x1b[1;5Ab"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := readAll(t, tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestModifiedKeyReaderSplitSequence guards that a sequence split across reads
// is still translated instead of being forwarded in pieces.
func TestModifiedKeyReaderSplitSequence(t *testing.T) {
	r := newModifiedKeyReader(&chunkReader{chunks: []string{"\x1b[27", ";2;13~hello"}})
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if got := out.String(); got != "\nhello" {
		t.Errorf("got %q, want %q", got, "\nhello")
	}
}

// TestModifiedKeyInputIsTerminalFile guards the regression where wrapping stdin
// in a plain io.Reader hid its file descriptor: Bubble Tea then skipped raw
// mode and the keyboard stopped working entirely.
func TestModifiedKeyInputIsTerminalFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "input")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck

	in := newModifiedKeyInput(f)
	if _, ok := any(in).(term.File); !ok {
		t.Fatal("modifiedKeyInput must implement term.File so Bubble Tea enables raw mode")
	}
	if got, want := in.Fd(), f.Fd(); got != want {
		t.Errorf("Fd() = %d, want the underlying %d", got, want)
	}

	if _, err := f.WriteString("\x1b[27;2;13~"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 8)
	n, err := in.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := string(buf[:n]); got != "\n" {
		t.Errorf("read %q, want %q", got, "\n")
	}
}

// TestModifiedEnterPipeline drives the whole chain — raw terminal bytes, the
// extended-key translator, Bubble Tea's parser and the key handler — and checks
// that a modified Enter inserts a newline instead of submitting.
func TestModifiedEnterPipeline(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"shift+enter", "abc\x1b[27;2;13~def"},
		{"alt+enter", "abc\x1b[27;3;13~def"},
		{"ctrl+enter", "abc\x1b[27;5;13~def"},
		{"ctrl+j", "abc\ndef"},
		{"kitty shift+enter", "abc\x1b[13;2udef"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			in := strings.NewReader(tc.in + "\x03") // Ctrl+C quits when idle
			p := tea.NewProgram(m, tea.WithInput(newModifiedKeyReader(in)),
				tea.WithoutRenderer(), tea.WithoutSignalHandler())
			final, err := p.Run()
			if err != nil {
				t.Fatal(err)
			}
			got := final.(*chatModel)
			if want := "abc\ndef"; got.input.Value() != want {
				t.Errorf("draft = %q, want %q", got.input.Value(), want)
			}
			if len(got.messages) != 1 {
				t.Errorf("messages = %d, want only the welcome banner — the draft was submitted", len(got.messages))
			}
		})
	}
}

// TestArrowKeysMoveWithinDraft guards that Up/Down move the cursor between
// draft lines and only fall back to command history at the boundaries.
func TestArrowKeysMoveWithinDraft(t *testing.T) {
	m := newTestModel(t)
	m.pushHistory("older")
	m.input.SetValue("first\nsecond")
	if got := m.input.Line(); got != 1 {
		t.Fatalf("cursor row = %d, want 1", got)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(*chatModel)
	if got := m.input.Line(); got != 0 {
		t.Errorf("cursor row after Up = %d, want 0", got)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(*chatModel)
	if got := m.input.Line(); got != 1 {
		t.Errorf("cursor row after Down = %d, want 1", got)
	}

	// Up on the first row goes back to history instead of moving the cursor.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(*chatModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(*chatModel)
	if got := m.input.Value(); got != "older" {
		t.Errorf("draft after history recall = %q, want %q", got, "older")
	}
}

// TestInputBoxKeepsEarlierLinesAfterNewline guards the regression where the
// first line scrolled out of the box after the first line break.
func TestInputBoxKeepsEarlierLinesAfterNewline(t *testing.T) {
	m := newTestModel(t)
	m.input.SetValue("abc")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = updated.(*chatModel)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("def")})
	m = updated.(*chatModel)

	box, rows := m.inputBlock()
	if rows != 2 {
		t.Fatalf("input rows = %d, want 2", rows)
	}
	flat := stripANSI(box)
	if !strings.Contains(flat, "abc") {
		t.Errorf("first line is missing from the input box:\n%s", flat)
	}
	if !strings.Contains(flat, "def") {
		t.Errorf("second line is missing from the input box:\n%s", flat)
	}
}

func TestControlByte(t *testing.T) {
	cases := []struct {
		code int
		want byte
		ok   bool
	}{
		{'a', 0x01, true},
		{'j', 0x0a, true},
		{'z', 0x1a, true},
		{'A', 0x01, true},
		{'Z', 0x1a, true},
		{32, 0x00, true},
		{63, 0x7f, true},
		{91, 0x1b, true},
		{95, 0x1f, true},
		{'1', 0, false},
		{13, 0, false},
		{127, 0, false},
	}
	for _, tc := range cases {
		got, ok := controlByte(tc.code)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("controlByte(%d) = %#x, %v; want %#x, %v", tc.code, got, ok, tc.want, tc.ok)
		}
	}
}

func TestLegacyKey(t *testing.T) {
	cases := []struct {
		code int
		want string
		ok   bool
	}{
		{9, "\t", true},
		{13, "\r", true},
		{27, "\x1b", true},
		{127, "\x7f", true},
		{'a', "a", true},
		{1, "", false},
	}
	for _, tc := range cases {
		got, ok := legacyKey(tc.code)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("legacyKey(%d) = %q, %v; want %q, %v", tc.code, got, ok, tc.want, tc.ok)
		}
	}
}

func TestModifiedEnterInsertsNewline(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"alt+enter", tea.KeyMsg{Type: tea.KeyEnter, Alt: true}},
		{"ctrl+j", tea.KeyMsg{Type: tea.KeyCtrlJ}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.input.SetValue("line one")

			updated, _ := m.Update(tc.msg)
			m = updated.(*chatModel)

			if got := m.input.Value(); got != "line one\n" {
				t.Errorf("draft = %q, want %q", got, "line one\n")
			}
			if m.state != tuiIdle {
				t.Errorf("state = %v, want idle — the draft was submitted", m.state)
			}
			if len(m.messages) != 1 {
				t.Errorf("messages = %d, want only the welcome banner", len(m.messages))
			}
		})
	}
}

// TestEscThenEnterInsertsNewline guards terminals that send Alt+Enter as a
// separate Esc followed by Enter.
func TestEscThenEnterInsertsNewline(t *testing.T) {
	m := newTestModel(t)
	m.input.SetValue("draft")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(*chatModel)
	if !m.altPending {
		t.Fatal("Esc did not arm altPending")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*chatModel)
	if got := m.input.Value(); got != "draft\n" {
		t.Errorf("draft = %q, want %q", got, "draft\n")
	}
	if m.altPending {
		t.Error("altPending was not cleared")
	}
}

// TestPlainEnterSubmits guards that the modifier handling did not break normal
// submission.
func TestPlainEnterSubmits(t *testing.T) {
	m := newTestModel(t)
	m.input.SetValue("/help")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(*chatModel)

	if len(m.messages) != 2 {
		t.Fatalf("messages = %d, want the banner plus the help text", len(m.messages))
	}
	if m.input.Value() != "" {
		t.Errorf("draft = %q, want it cleared after submit", m.input.Value())
	}
}
