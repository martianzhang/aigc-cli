package webfetch

import "testing"

func FuzzParseHeader(f *testing.F) {
	f.Add("X-Test: value")
	f.Add("Accept: text/html;q=0.9")
	f.Add("no-colon")
	f.Add(": empty-name")
	f.Add("Ünicode: ✓")

	f.Fuzz(func(t *testing.T, line string) {
		h, err := ParseHeader(line)
		if err != nil {
			return
		}
		if h.Name == "" {
			t.Fatalf("ParseHeader(%q) returned an empty name", line)
		}
	})
}
