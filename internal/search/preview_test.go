package search

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPreviewAppliesInjectJSON(t *testing.T) {
	deepseek := NewDeepSeekProvider("deepseek-secret-1111")
	deepseek.baseURL = "https://api.deepseek.test"

	t.Run("post body merges inject", func(t *testing.T) {
		plan, err := deepseek.Preview("q", 5, Options{
			Extra: json.RawMessage(`{"count":9,"thinking":{"enabled":true}}`),
		})
		if err != nil {
			t.Fatalf("Preview: %v", err)
		}
		curl := plan.RenderCurls("")
		for _, want := range []string{`"count":9`, `"thinking":{"enabled":true}`, `"query":"q"`} {
			if !strings.Contains(curl, want) {
				t.Errorf("curl missing injected field %q:\n%s", want, curl)
			}
		}
	})

	t.Run("get query merges inject", func(t *testing.T) {
		plan, err := NewBraveProvider("brave-secret-1234").Preview("q", 5, Options{
			Extra: json.RawMessage(`{"safesearch":"strict","extra_boost":2}`),
		})
		if err != nil {
			t.Fatalf("Preview: %v", err)
		}
		curl := plan.RenderCurls("")
		for _, want := range []string{"safesearch=strict", "extra_boost=2", "q=q", "count=5"} {
			if !strings.Contains(curl, want) {
				t.Errorf("curl missing injected query param %q:\n%s", want, curl)
			}
		}
	})

	t.Run("nested inject rejected for get", func(t *testing.T) {
		_, err := NewBraveProvider("k").Preview("q", 5, Options{
			Extra: json.RawMessage(`{"nested":{"a":1}}`),
		})
		if err == nil {
			t.Fatal("Preview accepted a nested GET inject")
		}
	})

	t.Run("duckduckgo rejects inject", func(t *testing.T) {
		_, err := NewDDGProvider().Preview("q", 5, Options{Extra: json.RawMessage(`{"a":1}`)})
		if err == nil {
			t.Fatal("DDG Preview accepted --inject-json")
		}
	})
}

func TestFetchPreviewsRenderCurl(t *testing.T) {
	deepseek := NewDeepSeekProvider("deepseek-secret-1111")
	deepseek.baseURL = "https://api.deepseek.test"
	exa := NewExaProvider("exa-secret-2222")
	exa.baseURL = "https://api.exa.test"

	tests := []struct {
		name    string
		preview FetchPreviewer
		wantURL string
		headers []string
		body    []string
		fullKey string
	}{
		{
			name:    "deepseek",
			preview: deepseek,
			wantURL: "https://api.deepseek.test/v1/tools/fetch",
			headers: []string{"Authorization: Bearer ...1111"},
			body:    []string{`"url":"https://example.com/a"`},
			fullKey: "deepseek-secret-1111",
		},
		{
			name:    "exa",
			preview: exa,
			wantURL: "https://api.exa.test/contents",
			headers: []string{"x-api-key: ...2222"},
			body:    []string{`"urls":["https://example.com/a"]`, `"text":true`},
			fullKey: "exa-secret-2222",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := tc.preview.PreviewFetch("https://example.com/a")
			if err != nil {
				t.Fatalf("PreviewFetch: %v", err)
			}
			curl := plan.RenderCurls("")
			if !strings.Contains(curl, tc.wantURL) {
				t.Errorf("curl missing URL %q:\n%s", tc.wantURL, curl)
			}
			for _, header := range tc.headers {
				if !strings.Contains(curl, header) {
					t.Errorf("curl missing header %q:\n%s", header, curl)
				}
			}
			for _, body := range tc.body {
				if !strings.Contains(curl, body) {
					t.Errorf("curl missing body fragment %q:\n%s", body, curl)
				}
			}
			if strings.Contains(curl, tc.fullKey) {
				t.Errorf("curl leaked the API key %q:\n%s", tc.fullKey, curl)
			}
		})
	}
}

func TestProviderPreviewsRenderCurl(t *testing.T) {
	deepseek := NewDeepSeekProvider("deepseek-secret-1111")
	deepseek.baseURL = "https://api.deepseek.test"
	exa := NewExaProvider("exa-secret-2222")
	exa.baseURL = "https://api.exa.test"

	tests := []struct {
		name       string
		previewer  Previewer
		wantURL    string
		wantHeader string
		wantBody   []string
		fullKey    string
	}{
		{
			name:       "duckduckgo",
			previewer:  NewDDGProvider(),
			wantURL:    "https://html.duckduckgo.com/html/?q=go+context",
			wantHeader: "User-Agent: Mozilla/5.0",
		},
		{
			name:       "brave",
			previewer:  NewBraveProvider("brave-secret-1234"),
			wantURL:    "https://api.search.brave.com/res/v1/web/search?q=go+context&count=5",
			wantHeader: "X-Subscription-Token: ...1234",
			fullKey:    "brave-secret-1234",
		},
		{
			name:       "firecrawl",
			previewer:  NewFirecrawlProvider("firecrawl-secret-5678"),
			wantURL:    "https://api.firecrawl.dev/v1/search",
			wantHeader: "Authorization: Bearer ...5678",
			wantBody:   []string{`"query":"go context"`, `"limit":5`},
			fullKey:    "firecrawl-secret-5678",
		},
		{
			name:       "deepseek",
			previewer:  deepseek,
			wantURL:    "https://api.deepseek.test/v1/tools/search",
			wantHeader: "Authorization: Bearer ...1111",
			wantBody:   []string{`"query":"go context"`, `"count":5`},
			fullKey:    "deepseek-secret-1111",
		},
		{
			name:       "exa",
			previewer:  exa,
			wantURL:    "https://api.exa.test/search",
			wantHeader: "x-api-key: ...2222",
			wantBody:   []string{`"query":"go context"`, `"numResults":5`},
			fullKey:    "exa-secret-2222",
		},
		{
			name:       "doubao",
			previewer:  NewDoubaoProvider("doubao-secret-3333"),
			wantURL:    "https://open.feedcoopapi.com/search_api/web_search",
			wantHeader: "Authorization: Bearer ...3333",
			wantBody:   []string{`"Query":"go context"`, `"Count":5`},
			fullKey:    "doubao-secret-3333",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := tc.previewer.Preview("go context", 5, Options{})
			if err != nil {
				t.Fatalf("Preview: %v", err)
			}
			curl := plan.RenderCurls("")
			if !strings.Contains(curl, tc.wantURL) {
				t.Errorf("curl missing URL %q:\n%s", tc.wantURL, curl)
			}
			if !strings.Contains(curl, tc.wantHeader) {
				t.Errorf("curl missing header %q:\n%s", tc.wantHeader, curl)
			}
			for _, body := range tc.wantBody {
				if !strings.Contains(curl, body) {
					t.Errorf("curl missing body fragment %q:\n%s", body, curl)
				}
			}
			if tc.fullKey != "" && strings.Contains(curl, tc.fullKey) {
				t.Errorf("curl leaked the API key %q:\n%s", tc.fullKey, curl)
			}
		})
	}
}
