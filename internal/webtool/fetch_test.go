package webtool

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsHTMLBody(t *testing.T) {
	tests := []struct {
		name string
		ct   string
		body string
		want bool
	}{
		{"text/html", "text/html; charset=utf-8", "<html></html>", true},
		{"xhtml", "application/xhtml+xml", "<html/>", true},
		{"json", "application/json", `{"a":1}`, false},
		{"plain text", "text/plain", "hello", false},
		{"xml feed", "application/xml", "<rss></rss>", false},
		{"sniff html doctype", "", "<!doctype html><html>", true},
		{"sniff html tag", "", "<html lang=\"en\">", true},
		{"sniff non-html", "", `{"a":1}`, false},
		{"blank body", "", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isHTMLBody(tc.ct, []byte(tc.body)); got != tc.want {
				t.Errorf("isHTMLBody(%q, %q) = %v, want %v", tc.ct, tc.body, got, tc.want)
			}
		})
	}
}

func TestTakeChars(t *testing.T) {
	t.Run("slices by rune and clamps", func(t *testing.T) {
		// Each rune is multi-byte; slicing by rune must never split one.
		chunk, total, start, end, remaining := TakeChars("αβγδε", 1, 2)
		if chunk != "βγ" || total != 5 || start != 1 || end != 3 || remaining != 2 {
			t.Errorf("got chunk=%q total=%d start=%d end=%d remaining=%d", chunk, total, start, end, remaining)
		}
	})

	t.Run("offset past end clamps", func(t *testing.T) {
		chunk, total, start, end, remaining := TakeChars("abc", 10, 5)
		if chunk != "" || total != 3 || start != 3 || end != 3 || remaining != 0 {
			t.Errorf("got chunk=%q total=%d start=%d end=%d remaining=%d", chunk, total, start, end, remaining)
		}
	})

	t.Run("negative offset clamps to zero", func(t *testing.T) {
		chunk, _, start, _, _ := TakeChars("abc", -5, 2)
		if chunk != "ab" || start != 0 {
			t.Errorf("got chunk=%q start=%d, want ab/0", chunk, start)
		}
	})
}

func TestFetch_convertsHTMLToMarkdown(t *testing.T) {
	allowLoopbackFetch(t)
	const page = `<!doctype html><html><head><title>Page Title</title></head><body>` +
		`<nav>NAVMARKER</nav><article><h1>Heading</h1>` +
		`<p>A substantial paragraph of body text so readability keeps it in the article.</p>` +
		`<p>Another paragraph with more meaningful prose for reliable extraction.</p>` +
		`</article><footer>FOOTMARKER</footer></body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, page)
	}))
	defer srv.Close()

	got := Fetch(`{"url":"` + srv.URL + `"}`)
	if !strings.Contains(got, "(markdown)") {
		t.Errorf("response not marked markdown:\n%s", got)
	}
	if !strings.Contains(got, "substantial paragraph") {
		t.Errorf("body text missing:\n%s", got)
	}
	if strings.Contains(got, "NAVMARKER") || strings.Contains(got, "FOOTMARKER") {
		t.Errorf("boilerplate not stripped:\n%s", got)
	}
}

func TestFetch_jsonPassthrough(t *testing.T) {
	allowLoopbackFetch(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"hello":"world"}`)
	}))
	defer srv.Close()

	got := Fetch(`{"url":"` + srv.URL + `"}`)
	if strings.Contains(got, "(markdown)") {
		t.Errorf("JSON should not be converted to markdown:\n%s", got)
	}
	if !strings.Contains(got, `{"hello":"world"}`) {
		t.Errorf("JSON body missing:\n%s", got)
	}
}

func TestFetch_pagination(t *testing.T) {
	allowLoopbackFetch(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, strings.Repeat("x", 100))
	}))
	defer srv.Close()

	got := Fetch(`{"url":"` + srv.URL + `","offset":10,"max_length":20}`)
	if !strings.Contains(got, "Showing chars 10-30 (70 remaining)") {
		t.Errorf("pagination window wrong:\n%s", got)
	}
	if !strings.Contains(got, "offset=30") {
		t.Errorf("missing next-offset hint:\n%s", got)
	}
}

func TestFetch_offsetPastEnd(t *testing.T) {
	allowLoopbackFetch(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, strings.Repeat("x", 100))
	}))
	defer srv.Close()

	got := Fetch(`{"url":"` + srv.URL + `","offset":1000}`)
	if !strings.Contains(got, "past the end") {
		t.Errorf("expected past-the-end message, got:\n%s", got)
	}
}

func TestFetch_invalidArgs(t *testing.T) {
	if got := Fetch("{bad json}"); !strings.Contains(got, "invalid arguments") {
		t.Errorf("Fetch(bad json) = %q", got)
	}
	if got := Fetch(`{}`); !strings.Contains(got, "url is required") {
		t.Errorf("Fetch(no url) = %q", got)
	}
	if got := Fetch(`{"url":"https://x.example","offset":-1}`); !strings.Contains(got, "offset must be non-negative") {
		t.Errorf("Fetch(negative offset) = %q", got)
	}
}

// allowLoopbackFetch disables the SSRF guard for a test that serves from a
// loopback httptest server.
func allowLoopbackFetch(t *testing.T) {
	t.Helper()
	prev := fetchURLGuard
	fetchURLGuard = func(string) error { return nil }
	t.Cleanup(func() { fetchURLGuard = prev })
}

// TestFetch_rejectsPrivateURL verifies the SSRF guard runs before any request is
// issued: no server is started, so a regression would hang or error differently
// rather than reaching these targets.
func TestFetch_rejectsPrivateURL(t *testing.T) {
	for _, u := range []string{
		"http://127.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/",
		"file:///etc/passwd",
	} {
		got := Fetch(`{"url":"` + u + `"}`)
		if !strings.HasPrefix(got, "Error:") {
			t.Errorf("Fetch(%s) = %q, want an Error:", u, got)
		}
	}
}
