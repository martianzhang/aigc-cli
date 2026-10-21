package web

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/webfetch"
)

// fakeFetchProvider satisfies search.Provider + search.Fetcher + FetchPreviewer.
type fakeFetchProvider struct {
	name    string
	title   string
	content string
	err     error

	called  bool
	lastURL string
	plan    *reqbuild.Plan
}

func (f *fakeFetchProvider) Name() string { return f.name }

func (f *fakeFetchProvider) Search(string, int) ([]search.Result, error) {
	return nil, nil
}

func (f *fakeFetchProvider) Fetch(url string) (string, string, error) {
	f.called = true
	f.lastURL = url
	return f.title, f.content, f.err
}

func (f *fakeFetchProvider) PreviewFetch(url string) (*reqbuild.Plan, error) {
	f.lastURL = url
	if f.plan != nil {
		return f.plan, nil
	}
	return &reqbuild.Plan{Method: http.MethodPost, URL: "https://" + f.name + ".example/fetch"}, nil
}

func newFetchRouter(providers ...search.Provider) *search.Router {
	router := search.NewRouter(search.NoopQuotaStore{})
	for _, p := range providers {
		router.Register(p.Name(), p, nil)
	}
	return router
}

func TestRunFetchBuiltinWritesBody(t *testing.T) {
	original := fetchDocument
	fetchDocument = func(context.Context, string, webfetch.Options) (*webfetch.Document, error) {
		return &webfetch.Document{Title: "T", Content: "# Body\n", FinalURL: "https://example.com/a"}, nil
	}
	t.Cleanup(func() { fetchDocument = original })

	var stdout, stderr bytes.Buffer
	if err := runFetch(&stdout, &stderr, "example.com/a", fetchFlags{}, newFetchRouter()); err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if stdout.String() != "# Body\n" {
		t.Errorf("stdout = %q, want only the body", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty without -v", stderr.String())
	}
}

func TestRunFetchBuiltinTruncates(t *testing.T) {
	originalDoc := fetchDocument
	fetchDocument = func(context.Context, string, webfetch.Options) (*webfetch.Document, error) {
		return &webfetch.Document{Content: "αβγδε"}, nil
	}
	originalVerbose := options.Shared.Verbose
	options.Shared.Verbose = true
	t.Cleanup(func() {
		fetchDocument = originalDoc
		options.Shared.Verbose = originalVerbose
	})

	var stdout, stderr bytes.Buffer
	if err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{maxChars: 3}, newFetchRouter()); err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if stdout.String() != "αβγ\n" {
		t.Errorf("stdout = %q, want three runes plus newline", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Truncated to 3 characters") {
		t.Errorf("stderr = %q, want truncation metadata", stderr.String())
	}
}

func TestRunFetchBuiltinDryRun(t *testing.T) {
	original := fetchDocument
	fetchDocument = func(context.Context, string, webfetch.Options) (*webfetch.Document, error) {
		t.Fatal("fetchDocument must not be called in dry-run")
		return nil, nil
	}
	t.Cleanup(func() { fetchDocument = original })

	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "example.com/a", fetchFlags{
		dryRun:  true,
		headers: []string{"User-Agent: custom-agent", "Accept:", "X-Test: 1"},
	}, newFetchRouter())
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}

	for _, want := range []string{
		"# dry run: no request sent",
		"# provider: built-in",
		"curl -X GET https://example.com/a",
		`-H "User-Agent: custom-agent"`,
		`-H "X-Test: 1"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout.String(), want)
		}
	}
	if strings.Contains(stdout.String(), `-H "Accept:`) {
		t.Errorf("stdout = %q, want Accept removed", stdout.String())
	}
}

func TestRunFetchHeaderWithProviderRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{
		provider: "fake",
		headers:  []string{"Authorization: Bearer x"},
	}, newFetchRouter(&fakeFetchProvider{name: "fake"}))
	if err == nil || !strings.Contains(err.Error(), "only supported by the built-in fetcher") {
		t.Fatalf("error = %v, want the -H/provider conflict", err)
	}
}

func TestRunFetchProvider(t *testing.T) {
	provider := &fakeFetchProvider{
		name:    "fake",
		title:   "Page",
		content: "αβγδε",
	}
	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "example.com/a", fetchFlags{
		provider: "fake",
		maxChars: 3,
	}, newFetchRouter(provider))
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if !provider.called || provider.lastURL != "https://example.com/a" {
		t.Errorf("provider = called:%v url:%q, want normalized URL", provider.called, provider.lastURL)
	}
	if stdout.String() != "αβγ\n" {
		t.Errorf("stdout = %q, want truncated provider content", stdout.String())
	}
}

func TestRunFetchProviderDryRun(t *testing.T) {
	provider := &fakeFetchProvider{name: "fake"}
	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{
		provider: "fake",
		dryRun:   true,
	}, newFetchRouter(provider))
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	for _, want := range []string{"# provider: fake", "curl -X POST https://fake.example/fetch"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout.String(), want)
		}
	}
	if provider.called {
		t.Error("provider was called in dry-run")
	}
}

func TestRunFetchSearchOnlyProviderFallsBack(t *testing.T) {
	original := fetchDocument
	fetchDocument = func(context.Context, string, webfetch.Options) (*webfetch.Document, error) {
		return &webfetch.Document{Content: "# builtin"}, nil
	}
	t.Cleanup(func() { fetchDocument = original })

	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{provider: "duckduckgo"},
		newFetchRouter(search.NewDDGProvider()))
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if stdout.String() != "# builtin\n" {
		t.Errorf("stdout = %q, want built-in body", stdout.String())
	}
	if !strings.Contains(stderr.String(), `provider "duckduckgo" does not support fetch`) {
		t.Errorf("stderr = %q, want a fallback note", stderr.String())
	}
}

func TestRunFetchStrategyWithoutFetcherFallsBack(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{
		provider: "auto",
		dryRun:   true,
	}, newFetchRouter())
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if !strings.Contains(stdout.String(), "# provider: built-in") {
		t.Errorf("stdout = %q, want a built-in curl preview", stdout.String())
	}
	if !strings.Contains(stderr.String(), `no fetch-capable provider matched "auto"`) {
		t.Errorf("stderr = %q, want a fallback note", stderr.String())
	}
}

func TestRunFetchSearchOnlyFallbackKeepsHeaders(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{
		provider: "duckduckgo",
		dryRun:   true,
		headers:  []string{"X-Test: 1"},
	}, newFetchRouter(search.NewDDGProvider()))
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if !strings.Contains(stdout.String(), `-H "X-Test: 1"`) {
		t.Errorf("stdout = %q, want the header on the built-in curl", stdout.String())
	}
}

func TestRunFetchUnknownProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{provider: "missing"},
		newFetchRouter(&fakeFetchProvider{name: "exa"}))
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("error = %v, want a not-configured error", err)
	}
}

func TestRunFetchProviderFailure(t *testing.T) {
	provider := &fakeFetchProvider{name: "fake", err: errors.New("boom")}
	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{provider: "fake"},
		newFetchRouter(provider))
	if err == nil || !strings.Contains(err.Error(), "fetch via fake") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v, want the provider failure wrapped", err)
	}
}

func TestRunFetchCookieDryRunMasksValues(t *testing.T) {
	dir := t.TempDir()
	jar := filepath.Join(dir, "cookies.txt")
	if err := os.WriteFile(jar, []byte(".example.com\tTRUE\t/\tFALSE\t0\tsession\tfile-secret\n"), 0o600); err != nil {
		t.Fatalf("write cookie file: %v", err)
	}
	out := filepath.Join(dir, "out.txt")

	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{
		dryRun:    true,
		cookies:   []string{"session=abc; theme=dark", jar},
		cookieJar: out,
	}, newFetchRouter())
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}

	for _, want := range []string{
		`-b "session=***; theme=***"`,
		`-b "` + jar + `"`,
		`-c "` + out + `"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout.String(), want)
		}
	}
	if strings.Contains(stdout.String(), "abc") || strings.Contains(stdout.String(), "file-secret") {
		t.Errorf("stdout = %q, leaked a cookie value", stdout.String())
	}
}

func TestRunFetchCookieHeaderWinsOverCookieFlag(t *testing.T) {
	original := fetchDocument
	var gotCookies []*http.Cookie
	fetchDocument = func(_ context.Context, rawURL string, opts webfetch.Options) (*webfetch.Document, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Fatalf("parse URL: %v", err)
		}
		gotCookies = opts.CookieJar.Cookies(u)
		return &webfetch.Document{Content: "ok", FinalURL: rawURL}, nil
	}
	t.Cleanup(func() { fetchDocument = original })

	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{
		cookies: []string{"session=from-flag"},
		headers: []string{"Cookie: session=from-header"},
	}, newFetchRouter())
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if len(gotCookies) != 1 || gotCookies[0].Value != "from-header" {
		t.Errorf("cookies = %+v, want the explicit header value", gotCookies)
	}
}

func TestRunFetchCookieHeaderEmptyClearsCookies(t *testing.T) {
	original := fetchDocument
	var gotCookies []*http.Cookie
	fetchDocument = func(_ context.Context, rawURL string, opts webfetch.Options) (*webfetch.Document, error) {
		u, _ := url.Parse(rawURL)
		gotCookies = opts.CookieJar.Cookies(u)
		return &webfetch.Document{Content: "ok", FinalURL: rawURL}, nil
	}
	t.Cleanup(func() { fetchDocument = original })

	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{
		cookies: []string{"session=from-flag"},
		headers: []string{"Cookie:"},
	}, newFetchRouter())
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if len(gotCookies) != 0 {
		t.Errorf("cookies = %+v, want none", gotCookies)
	}
}

func TestRunFetchCookieJarWritesResponseCookies(t *testing.T) {
	original := fetchDocument
	fetchDocument = func(_ context.Context, rawURL string, opts webfetch.Options) (*webfetch.Document, error) {
		u, _ := url.Parse(rawURL)
		opts.CookieJar.SetCookies(u, []*http.Cookie{{Name: "new", Value: "1"}})
		return &webfetch.Document{Content: "ok", FinalURL: rawURL}, nil
	}
	t.Cleanup(func() { fetchDocument = original })

	jar := filepath.Join(t.TempDir(), "out.txt")
	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{
		cookies:   []string{"session=abc"},
		cookieJar: jar,
	}, newFetchRouter())
	if err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	data, err := os.ReadFile(jar)
	if err != nil {
		t.Fatalf("read jar: %v", err)
	}
	for _, want := range []string{"session\tabc", "new\t1"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("jar = %q, want %q", data, want)
		}
	}
}

func TestRunFetchProviderRejectsCookies(t *testing.T) {
	provider := &fakeFetchProvider{name: "fake"}
	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{
		provider: "fake",
		cookies:  []string{"session=abc"},
	}, newFetchRouter(provider))
	if err == nil || !strings.Contains(err.Error(), "--cookie") {
		t.Fatalf("error = %v, want a cookie/provider conflict", err)
	}
}

func TestRunFetchLocalFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(`<html><head><title>Local</title></head><body><article><h1>Hello</h1><p>Local body.</p></article></body></html>`), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := runFetch(&stdout, &stderr, path, fetchFlags{}, newFetchRouter()); err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	for _, want := range []string{"Hello", "Local body"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout.String(), want)
		}
	}
}

func TestRunFetchLocalFileRejectsNetworkFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte("<html></html>"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, path, fetchFlags{provider: "deepseek"}, newFetchRouter())
	if err == nil || !strings.Contains(err.Error(), "local files") {
		t.Fatalf("error = %v, want a local-file/network-flag conflict", err)
	}
}

func TestRunFetchLocalFileDryRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte("<html></html>"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := runFetch(&stdout, &stderr, path, fetchFlags{dryRun: true}, newFetchRouter()); err != nil {
		t.Fatalf("runFetch: %v", err)
	}
	if !strings.Contains(stdout.String(), "# source: local file") {
		t.Errorf("stdout = %q, want a local-file dry-run note", stdout.String())
	}
	if strings.Contains(stdout.String(), "curl") {
		t.Errorf("stdout = %q, want no curl for a local file", stdout.String())
	}
}

func TestWriteFetchedContentAddsNewline(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := writeFetchedContent(&stdout, &stderr, "abc", 0); err != nil {
		t.Fatalf("writeFetchedContent: %v", err)
	}
	if stdout.String() != "abc\n" {
		t.Errorf("stdout = %q, want a trailing newline", stdout.String())
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		max       int
		want      string
		truncated bool
	}{
		{name: "disabled", content: "abcdef", max: 0, want: "abcdef"},
		{name: "short enough", content: "abc", max: 5, want: "abc"},
		{name: "truncate", content: "abcdef", max: 3, want: "abc", truncated: true},
		{name: "multi-byte", content: "αβγδε", max: 2, want: "αβ", truncated: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, truncated := truncateContent(tc.content, tc.max)
			if got != tc.want || truncated != tc.truncated {
				t.Errorf("truncateContent(%q, %d) = %q/%v, want %q/%v",
					tc.content, tc.max, got, truncated, tc.want, tc.truncated)
			}
		})
	}
}
