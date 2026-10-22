package web

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/search"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestProviderCompletionNames(t *testing.T) {
	providers := map[string]*types.WebSearchProvider{
		"deepseek": {Type: "deepseek", APIKey: "k"},
		"exa":      {Type: "exa", APIKey: "k"},
		"brave":    {Type: "brave"}, // unavailable, must not be suggested
	}

	tests := []struct {
		name      string
		fetchOnly bool
		prefix    string
		want      []string
	}{
		{name: "search all", want: []string{"deepseek", "duckduckgo", "exa"}},
		{name: "search prefix", prefix: "d", want: []string{"deepseek", "duckduckgo"}},
		{name: "search prefix case-insensitive", prefix: "D", want: []string{"deepseek", "duckduckgo"}},
		{name: "fetch only", fetchOnly: true, want: []string{"deepseek", "exa"}},
		{name: "fetch prefix", fetchOnly: true, prefix: "e", want: []string{"exa"}},
		{name: "no match", prefix: "zzz", want: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := providerCompletionNames(providers, tc.fetchOnly, tc.prefix)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("providerCompletionNames() = %v, want %v", got, tc.want)
			}
		})
	}

	if got := providerCompletionNames(nil, true, ""); len(got) != 0 {
		t.Errorf("fetch completion with nil config = %v, want none", got)
	}
}

func TestWriteProviderList(t *testing.T) {
	router := search.NewRouter()
	router.RegisterWithType("duckduckgo", search.NewDDGProvider(), "duckduckgo")
	router.RegisterWithType("deepseek", &fakeFetchProvider{name: "deepseek"}, "deepseek")
	router.RegisterWithType("pixabay", search.NewPixabayProvider("k"), "pixabay")
	router.RegisterUnavailable("brave", "brave", errors.New("brave requires api_key"))

	var buf bytes.Buffer
	if err := writeProviderList(&buf, router); err != nil {
		t.Fatalf("writeProviderList: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"NAME", "TYPE", "SEARCH", "FETCH", "MEDIA", "STATUS",
		"duckduckgo", "deepseek", "brave", "pixabay",
		"image,photo,illustration,vector,video",
		"unavailable: brave requires api_key",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "deepseek") {
		t.Fatalf("deepseek row missing:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "deepseek") {
			fields := strings.Fields(line)
			if len(fields) < 5 || fields[3] != "yes" {
				t.Errorf("deepseek row missing fetch capability:\n%s", out)
			}
		}
	}
}

func TestSearchUnavailableProviderReason(t *testing.T) {
	router := search.NewRouter()
	router.RegisterUnavailable("brave", "brave", errors.New("brave requires api_key"))

	var stdout bytes.Buffer
	err := runSearch(&stdout, searchRequest{query: "q", provider: "brave", limit: 5}, router)
	if err == nil || !strings.Contains(err.Error(), "configured but unavailable") || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("error = %v, want configured-but-unavailable reason", err)
	}
}

func TestFetchUnavailableProviderReason(t *testing.T) {
	router := search.NewRouter()
	router.RegisterUnavailable("deepseek", "deepseek", errors.New("deepseek requires api_key"))

	var stdout, stderr bytes.Buffer
	err := runFetch(&stdout, &stderr, "https://example.com/a", fetchFlags{provider: "deepseek"}, router)
	if err == nil || !strings.Contains(err.Error(), "configured but unavailable") || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("error = %v, want configured-but-unavailable reason", err)
	}
}
