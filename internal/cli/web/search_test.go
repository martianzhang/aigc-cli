package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// fakeProvider is an in-memory search.Provider that records its inputs.
type fakeProvider struct {
	name    string
	results []search.Result
	err     error

	calls     int
	lastQuery string
	lastLimit int
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Search(query string, limit int) ([]search.Result, error) {
	f.calls++
	f.lastQuery = query
	f.lastLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

// fakeRawProvider adds the optional RawProvider extension to the test double.
type fakeRawProvider struct {
	fakeProvider
	raw       json.RawMessage
	lastOpts  search.Options
	rawCalled bool
}

func (f *fakeRawProvider) SearchRaw(query string, limit int, opts search.Options) ([]search.Result, json.RawMessage, error) {
	f.rawCalled = true
	f.lastOpts = opts
	results, err := f.Search(query, limit)
	return results, f.raw, err
}

// fakePreviewProvider adds the optional Previewer extension for --dry-run.
type fakePreviewProvider struct {
	fakeProvider
	plan     *reqbuild.Plan
	lastOpts search.Options
}

func (f *fakePreviewProvider) Preview(_ string, _ int, opts search.Options) (*reqbuild.Plan, error) {
	f.lastOpts = opts
	return f.plan, nil
}

func newFakeRouter(providers ...search.Provider) *search.Router {
	router := search.NewRouter()
	for _, p := range providers {
		router.Register(p.Name(), p)
	}
	return router
}

func TestResolveProvider(t *testing.T) {
	original := options.Shared.Cfg
	t.Cleanup(func() { options.Shared.Cfg = original })

	tests := []struct {
		name   string
		cli    string
		shared string
		want   string
	}{
		{name: "cli wins", cli: "brave", shared: "deepseek", want: "brave"},
		{name: "shared default", shared: "deepseek", want: "deepseek"},
		{name: "duckduckgo default", want: "duckduckgo"},
		{name: "blank cli uses shared", cli: "  ", shared: "firecrawl", want: "firecrawl"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &types.Config{}
			if tc.shared != "" {
				cfg.Defaults = &types.ConfigDefaults{Search: &types.SearchDefaults{Provider: tc.shared}}
			}
			options.Shared.Cfg = cfg
			if got := resolveProvider(tc.cli); got != tc.want {
				t.Errorf("resolveProvider(%q) = %q, want %q", tc.cli, got, tc.want)
			}
		})
	}
}

func TestNormalizeLimit(t *testing.T) {
	tests := []struct {
		in   int
		want int
	}{
		{in: 0, want: defaultSearchLimit},
		{in: -3, want: defaultSearchLimit},
		{in: 1, want: 1},
		{in: 20, want: 20},
		{in: 99, want: maxSearchLimit},
	}

	for _, tc := range tests {
		if got := normalizeLimit(tc.in); got != tc.want {
			t.Errorf("normalizeLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestBuildSearchRequestPrecedence(t *testing.T) {
	withStdin(t, false, "")
	tests := []struct {
		name       string
		args       []string
		jsonInput  string
		inject     string
		setFlags   map[string]string
		wantQuery  string
		wantProv   string
		wantLimit  int
		wantInject string
	}{
		{
			name:      "positional args",
			args:      []string{"golang", "context"},
			wantQuery: "golang context",
			wantLimit: defaultSearchLimit,
		},
		{
			name:      "json supplies query provider limit",
			jsonInput: `{"query":"golang","provider":"deepseek","limit":7}`,
			wantQuery: "golang",
			wantProv:  "deepseek",
			wantLimit: 7,
		},
		{
			name:      "positional args beat json query",
			args:      []string{"from", "args"},
			jsonInput: `{"query":"from json","limit":3}`,
			wantQuery: "from args",
			wantLimit: 3,
		},
		{
			name:      "cli provider and limit beat json",
			jsonInput: `{"query":"q","provider":"exa","limit":3}`,
			setFlags:  map[string]string{"provider": "brave", "limit": "9"},
			wantQuery: "q",
			wantProv:  "brave",
			wantLimit: 9,
		},
		{
			name:       "inject json is parsed",
			args:       []string{"q"},
			inject:     `{"type":"neural"}`,
			wantQuery:  "q",
			wantLimit:  defaultSearchLimit,
			wantInject: `{"type":"neural"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := SearchCmd()
			flags := &searchFlags{jsonInput: tc.jsonInput, injectJSON: tc.inject}
			for name, value := range tc.setFlags {
				if err := cmd.Flags().Set(name, value); err != nil {
					t.Fatalf("set %s: %v", name, err)
				}
			}
			// Refresh the flag-bound struct from the flag set, mirroring RunE.
			flags.provider, _ = cmd.Flags().GetString("provider")
			flags.limit, _ = cmd.Flags().GetInt("limit")

			req, err := buildSearchRequest(cmd, tc.args, flags)
			if err != nil {
				t.Fatalf("buildSearchRequest: %v", err)
			}
			if req.query != tc.wantQuery || req.provider != tc.wantProv || req.limit != tc.wantLimit {
				t.Errorf("request = %+v, want query=%q provider=%q limit=%d",
					req, tc.wantQuery, tc.wantProv, tc.wantLimit)
			}
			if tc.wantInject != "" && string(req.inject) != tc.wantInject {
				t.Errorf("inject = %s, want %s", req.inject, tc.wantInject)
			}
		})
	}
}

// withStdin replaces the stdin detector/reader for one test.
func withStdin(t *testing.T, piped bool, text string) {
	t.Helper()
	oldPipe, oldRead := stdinIsPipe, readStdin
	stdinIsPipe = func() bool { return piped }
	readStdin = func() ([]byte, error) { return []byte(text), nil }
	t.Cleanup(func() {
		stdinIsPipe = oldPipe
		readStdin = oldRead
	})
}

func TestBuildSearchRequestReadsQueryFromPipe(t *testing.T) {
	withStdin(t, true, "golang\ncontext\ntimeout\n")

	req, err := buildSearchRequest(SearchCmd(), nil, &searchFlags{})
	if err != nil {
		t.Fatalf("buildSearchRequest: %v", err)
	}
	if req.query != "golang context timeout" {
		t.Errorf("query = %q, want the flattened stdin text", req.query)
	}
}

func TestBuildSearchRequestExplicitStdinDash(t *testing.T) {
	withStdin(t, true, "golang context")

	req, err := buildSearchRequest(SearchCmd(), []string{"-"}, &searchFlags{})
	if err != nil {
		t.Fatalf("buildSearchRequest: %v", err)
	}
	if req.query != "golang context" {
		t.Errorf("query = %q, want stdin", req.query)
	}
}

func TestBuildSearchRequestPositionalArgsBeatPipe(t *testing.T) {
	withStdin(t, true, "from stdin")
	oldRead := readStdin
	readStdin = func() ([]byte, error) {
		t.Fatal("stdin must not be read when a positional query is present")
		return nil, nil
	}
	t.Cleanup(func() { readStdin = oldRead })

	req, err := buildSearchRequest(SearchCmd(), []string{"from", "args"}, &searchFlags{})
	if err != nil {
		t.Fatalf("buildSearchRequest: %v", err)
	}
	if req.query != "from args" {
		t.Errorf("query = %q, want positional args", req.query)
	}
}

func TestBuildSearchRequestEmptyStdin(t *testing.T) {
	withStdin(t, true, "   \n")

	_, err := buildSearchRequest(SearchCmd(), []string{"-"}, &searchFlags{})
	if err == nil || !strings.Contains(err.Error(), "stdin is empty") {
		t.Fatalf("error = %v, want an empty-stdin error", err)
	}
}

func TestBuildSearchRequestRejectsJSONFromStdin(t *testing.T) {
	_, err := buildSearchRequest(SearchCmd(), nil, &searchFlags{jsonInput: "-"})
	if err == nil || !strings.Contains(err.Error(), "--json does not read stdin") {
		t.Fatalf("error = %v, want a --json stdin rejection", err)
	}
}

func TestBuildSearchRequestErrors(t *testing.T) {
	withStdin(t, false, "")
	tests := []struct {
		name      string
		flags     searchFlags
		wantError string
	}{
		{name: "missing query", wantError: "query is required"},
		{name: "invalid json", flags: searchFlags{jsonInput: `{oops`}, wantError: "parse --json"},
		{name: "unknown json field", flags: searchFlags{jsonInput: `{"query":"q","provder":"x"}`}, wantError: "parse --json"},
		{name: "inject is not an object", flags: searchFlags{jsonInput: `{"query":"q"}`, injectJSON: `[1,2]`}, wantError: "inject-json"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildSearchRequest(SearchCmd(), nil, &tc.flags)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantError)
			}
		})
	}
}

func TestRunSearchPrintsRawProviderJSON(t *testing.T) {
	provider := &fakeRawProvider{
		fakeProvider: fakeProvider{
			name:    "deepseek",
			results: []search.Result{{Title: "Go", URL: "https://go.dev"}},
		},
		raw: json.RawMessage(`{"results":[{"title":"Go","url":"https://go.dev"}],"request_id":"abc"}`),
	}
	var stdout bytes.Buffer

	err := runSearch(&stdout, searchRequest{
		query:    "golang context",
		provider: "deepseek",
		limit:    maxSearchLimit,
	}, newFakeRouter(provider))
	if err != nil {
		t.Fatalf("runSearch: %v", err)
	}

	if provider.lastQuery != "golang context" || provider.lastLimit != maxSearchLimit {
		t.Errorf("provider got query=%q limit=%d, want %q/%d",
			provider.lastQuery, provider.lastLimit, "golang context", maxSearchLimit)
	}

	var out map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v; raw = %q", err, stdout.String())
	}
	if out["request_id"] != "abc" {
		t.Errorf("output = %q, want the provider's untouched response", stdout.String())
	}
	if !strings.Contains(stdout.String(), "\n  \"request_id\": \"abc\"") {
		t.Errorf("output = %q, want pretty-printed JSON", stdout.String())
	}
	if strings.Contains(stdout.String(), `"query"`) {
		t.Errorf("output = %q, want no CLI wrapper around the vendor response", stdout.String())
	}
}

func TestRunSearchPassesInjectJSON(t *testing.T) {
	provider := &fakeRawProvider{
		fakeProvider: fakeProvider{name: "exa"},
		raw:          json.RawMessage(`{"ok":true}`),
	}
	inject := json.RawMessage(`{"type":"neural"}`)

	var stdout bytes.Buffer
	if err := runSearch(&stdout, searchRequest{
		query:    "q",
		provider: "exa",
		limit:    5,
		inject:   inject,
	}, newFakeRouter(provider)); err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	if !provider.rawCalled {
		t.Fatal("SearchRaw was not called")
	}
	if string(provider.lastOpts.Extra) != string(inject) {
		t.Errorf("inject = %s, want %s", provider.lastOpts.Extra, inject)
	}
}

func TestRunSearchFallsBackToResultsWithoutRaw(t *testing.T) {
	provider := &fakeProvider{
		name:    "brave",
		results: []search.Result{{Title: "Go", URL: "https://go.dev"}},
	}
	var stdout bytes.Buffer

	if err := runSearch(&stdout, searchRequest{query: "q", provider: "brave", limit: 5}, newFakeRouter(provider)); err != nil {
		t.Fatalf("runSearch: %v", err)
	}

	var out []search.Result
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("output is not JSON: %v; raw = %q", err, stdout.String())
	}
	if len(out) != 1 || out[0].URL != "https://go.dev" {
		t.Errorf("output = %+v, want the normalized results as a JSON array", out)
	}
}

func TestRunSearchUnknownProvider(t *testing.T) {
	var stdout bytes.Buffer

	err := runSearch(&stdout, searchRequest{query: "q", provider: "missing", limit: 5}, newFakeRouter(
		&fakeProvider{name: "brave"},
		&fakeProvider{name: "duckduckgo"},
	))
	if err == nil {
		t.Fatal("runSearch returned nil error for an unknown provider")
	}
	for _, want := range []string{`"missing"`, "brave", "duckduckgo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err.Error(), want)
		}
	}
}

func TestRunSearchAllProvidersFail(t *testing.T) {
	var stdout bytes.Buffer

	err := runSearch(&stdout, searchRequest{query: "q", provider: "broken", limit: 5},
		newFakeRouter(&fakeProvider{name: "broken", err: errors.New("boom")}))
	if err == nil {
		t.Fatal("runSearch returned nil error when the provider failed")
	}
	if !strings.Contains(err.Error(), "search failed") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to wrap the provider failure", err.Error())
	}
}

func TestRunSearchDryRunPrintsCurlWithoutCallingProvider(t *testing.T) {
	provider := &fakePreviewProvider{
		fakeProvider: fakeProvider{name: "brave"},
		plan: &reqbuild.Plan{
			Method:  "GET",
			URL:     "https://api.example/search?q=golang+context",
			Headers: []reqbuild.Header{{Name: "X-Test", Value: "1"}},
		},
	}
	inject := json.RawMessage(`{"extra":"field"}`)
	var stdout bytes.Buffer

	err := runSearch(&stdout, searchRequest{
		query:    "golang context",
		provider: "brave",
		limit:    maxSearchLimit,
		inject:   inject,
		dryRun:   true,
	}, newFakeRouter(provider))
	if err != nil {
		t.Fatalf("runSearch: %v", err)
	}
	if provider.calls != 0 {
		t.Errorf("provider calls = %d, want 0 in dry-run", provider.calls)
	}
	if string(provider.lastOpts.Extra) != string(inject) {
		t.Errorf("preview inject = %s, want %s", provider.lastOpts.Extra, inject)
	}

	for _, want := range []string{
		"# dry run: no request sent",
		"# provider: brave",
		"curl -X GET https://api.example/search?q=golang+context",
		`-H "X-Test: 1"`,
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want it to contain %q", stdout.String(), want)
		}
	}
}

func TestRunSearchDryRunReportsMissingPreview(t *testing.T) {
	var stdout bytes.Buffer
	err := runSearch(&stdout, searchRequest{query: "q", provider: "plain", limit: 5, dryRun: true},
		newFakeRouter(&fakeProvider{name: "plain"}))
	if err == nil || !strings.Contains(err.Error(), "does not provide a curl preview") {
		t.Fatalf("error = %v, want a missing-preview error", err)
	}
}

func TestWriteRawJSONPrintsNonJSONVerbatim(t *testing.T) {
	var buf bytes.Buffer
	if err := writeRawJSON(&buf, &search.SearchResult{Raw: json.RawMessage("<html>error</html>")}); err != nil {
		t.Fatalf("writeRawJSON: %v", err)
	}
	if buf.String() != "<html>error</html>\n" {
		t.Errorf("output = %q, want the raw body with a trailing newline", buf.String())
	}
}

func TestWriteRawJSONNilResult(t *testing.T) {
	var buf bytes.Buffer
	if err := writeRawJSON(&buf, nil); err != nil {
		t.Fatalf("writeRawJSON: %v", err)
	}
	if buf.String() != "null\n" {
		t.Errorf("output = %q, want null", buf.String())
	}
}
