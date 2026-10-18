package chat

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pngMagic is the minimal PNG signature that http.DetectContentType recognizes.
var pngMagic = []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R'}

func TestExpandMentions(t *testing.T) {
	dir := t.TempDir()

	mustWrite := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
		return path
	}

	txtPath := mustWrite("a.txt", []byte("hello world"))
	imgPath := mustWrite("pic.png", pngMagic)
	binPath := mustWrite("a.bin", []byte{'a', 0x00, 'b'})
	secretPath := mustWrite("id_rsa", []byte("PRIVATE"))

	tests := []struct {
		name        string
		input       string
		deny        []string
		wantText    string // exact match when non-empty
		wantContain []string
		wantImages  int
		wantWarn    int
	}{
		{
			name:        "text file is inlined",
			input:       "explain @" + txtPath,
			wantContain: []string{`<file name="` + txtPath + `">`, "hello world", "</file>"},
		},
		{
			name:       "image becomes an attachment",
			input:      "what is @" + imgPath,
			wantText:   "what is [attached image: " + imgPath + "]",
			wantImages: 1,
		},
		{
			name:        "binary file is refused",
			input:       "look @" + binPath,
			wantText:    "look @" + binPath,
			wantWarn:    1,
			wantContain: []string{"binary file"},
		},
		{
			name:        "deny policy blocks the read",
			input:       "read @" + secretPath,
			deny:        []string{`^id_rsa$`},
			wantText:    "read @" + secretPath,
			wantWarn:    1,
			wantContain: []string{"read_deny"},
		},
		{
			name:     "missing file is left untouched",
			input:    "hello @does-not-exist-xyz",
			wantText: "hello @does-not-exist-xyz",
		},
		{
			name:     "email address is not a mention",
			input:    "mail user@example.com please",
			wantText: "mail user@example.com please",
		},
		{
			name:     "scoped package is not a mention",
			input:    "install @scope/pkg now",
			wantText: "install @scope/pkg now",
		},
		{
			name:       "multiple mentions",
			input:      "@" + txtPath + " and @" + imgPath,
			wantImages: 1,
			wantContain: []string{
				"hello world",
				"[attached image: " + imgPath + "]",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotText, gotImages, warnings := expandMentions(tc.input, tc.deny)
			if tc.wantText != "" && gotText != tc.wantText {
				t.Errorf("text = %q, want %q", gotText, tc.wantText)
			}
			if len(gotImages) != tc.wantImages {
				t.Errorf("images = %d, want %d", len(gotImages), tc.wantImages)
			}
			if len(warnings) != tc.wantWarn {
				t.Errorf("warnings = %v, want %d", warnings, tc.wantWarn)
			}
			for _, want := range tc.wantContain {
				haystack := gotText + "\n" + strings.Join(warnings, "\n")
				if !strings.Contains(haystack, want) {
					t.Errorf("output %q / %v, want it to contain %q", gotText, warnings, want)
				}
			}
		})
	}
}

func TestExpandMentionsOversizeImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.png")
	data := make([]byte, 0, maxImageBytes+1)
	data = append(data, pngMagic...)
	data = append(data, bytes.Repeat([]byte{0}, maxImageBytes)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	gotText, images, warnings := expandMentions("@"+path, nil)
	if gotText != "@"+path {
		t.Errorf("text = %q, want the untouched token", gotText)
	}
	if len(images) != 0 {
		t.Errorf("images = %d, want 0", len(images))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "exceeds") {
		t.Errorf("warnings = %v, want an oversize warning", warnings)
	}
}

func TestTrailingMentionStart(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"@", 0},
		{"@src", 0},
		{"look at @src/ma", 8},
		{"look at @src/ma ", -1},
		{"a@b", -1},
		{"email user@x.com", -1},
		{"no mention here", -1},
	}
	for _, tc := range tests {
		if got := trailingMentionStart(tc.input); got != tc.want {
			t.Errorf("trailingMentionStart(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCompleteMention_CyclesMatches(t *testing.T) {
	m := newChatModel(nil, nil, 10, "test-model", "", nil, false, 0, 0, 0)
	m.fileIndex = []string{"alpha.txt", "alpine.txt", "beta.md"}
	m.fileIndexReady = true
	m.cycleIdx = -1

	m.input.SetValue("read @al")
	if _, _ = m.completeMention("read @al", 5); m.input.Value() == "read @al" {
		t.Fatalf("first Tab did not change input: %q", m.input.Value())
	}
	if got := m.input.Value(); !strings.HasPrefix(got, "read @") {
		t.Errorf("after first Tab = %q, want the prefix preserved", got)
	}
	first := m.input.Value()

	m.completeMention(first, 5)
	if m.input.Value() == first {
		t.Error("second Tab did not advance the completion cycle")
	}
}
