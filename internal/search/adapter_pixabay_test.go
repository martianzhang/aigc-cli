package search

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNewPixabayProviderDefaults(t *testing.T) {
	p := NewPixabayProvider("pb-key")
	if p.Name() != "pixabay" {
		t.Errorf("Name() = %q, want pixabay", p.Name())
	}
	if p.baseURL != defaultPixabayBaseURL {
		t.Errorf("baseURL = %q, want %q", p.baseURL, defaultPixabayBaseURL)
	}
	if p.baseURL != "https://pixabay.com" {
		t.Errorf("default base URL = %q, want https://pixabay.com", p.baseURL)
	}
}

func TestPixabayProviderImplementsCapabilities(t *testing.T) {
	p := NewPixabayProvider("pb-key")
	if _, ok := any(p).(Provider); !ok {
		t.Error("PixabayProvider must implement Provider")
	}
	if _, ok := any(p).(RawProvider); !ok {
		t.Error("PixabayProvider must implement RawProvider")
	}
	if _, ok := any(p).(Previewer); !ok {
		t.Error("PixabayProvider must implement Previewer")
	}
	if _, ok := any(p).(MediaProvider); !ok {
		t.Error("PixabayProvider must implement MediaProvider")
	}
	if got := strings.Join(p.MediaTypes(), ","); got != "image,photo,illustration,vector,video" {
		t.Errorf("MediaTypes() = %q", got)
	}
}

func TestPixabaySearchImage(t *testing.T) {
	t.Setenv("AIGC_CLI_CACHE_DIR", t.TempDir())
	var gotPath string
	var gotQuery url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"total":2,"hits":[
			{"id":1,"pageURL":"https://pixabay.com/photos/a-1/","tags":"rain, window","user":"alice"},
			{"id":2,"pageURL":"https://pixabay.com/photos/b-2/","tags":"","user":"bob"}
		]}`)
	}))
	defer srv.Close()

	p := &PixabayProvider{apiKey: "pb-key", baseURL: srv.URL}
	results, raw, err := p.SearchRaw("rain window", 5, Options{})
	if err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}

	if gotPath != "/api/" {
		t.Errorf("path = %q, want /api/", gotPath)
	}
	if got := gotQuery.Get("key"); got != "pb-key" {
		t.Errorf("key = %q, want pb-key", got)
	}
	if got := gotQuery.Get("q"); got != "rain window" {
		t.Errorf("q = %q, want rain window", got)
	}
	if got := gotQuery.Get("per_page"); got != "5" {
		t.Errorf("per_page = %q, want 5", got)
	}
	if got := gotQuery.Get("image_type"); got != "" {
		t.Errorf("image_type = %q, want empty (vendor default)", got)
	}

	want := []Result{
		{URL: "https://pixabay.com/photos/a-1/", Title: "rain, window"},
		{URL: "https://pixabay.com/photos/b-2/", Title: "bob"},
	}
	if len(results) != len(want) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(want))
	}
	for i := range want {
		if results[i] != want[i] {
			t.Errorf("results[%d] = %+v, want %+v", i, results[i], want[i])
		}
	}
	if !strings.Contains(string(raw), `"pageURL"`) {
		t.Errorf("raw response was not returned verbatim: %s", raw)
	}
}

func TestPixabaySearchTypeAndInject(t *testing.T) {
	t.Setenv("AIGC_CLI_CACHE_DIR", t.TempDir())
	tests := []struct {
		name        string
		mediaType   string
		inject      string
		wantPath    string
		wantParams  map[string]string
		absentParam string
	}{
		{
			name:       "photo endpoint",
			mediaType:  "photo",
			wantPath:   "/api/",
			wantParams: map[string]string{"image_type": "photo"},
		},
		{
			name:        "video endpoint",
			mediaType:   "video",
			wantPath:    "/api/videos/",
			absentParam: "image_type",
		},
		{
			name:       "inject wins for vendor filters",
			mediaType:  "image",
			inject:     `{"orientation":"horizontal","min_width":1920,"page":2,"q":"hacked","per_page":99,"image_type":"vector","key":"stolen"}`,
			wantPath:   "/api/",
			wantParams: map[string]string{"orientation": "horizontal", "min_width": "1920", "page": "2", "q": "rain window", "per_page": "5", "image_type": "all", "key": "pb-key"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			var gotQuery url.Values
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotQuery = r.URL.Query()
				io.WriteString(w, `{"hits":[]}`)
			}))
			defer srv.Close()

			p := &PixabayProvider{apiKey: "pb-key", baseURL: srv.URL}
			opts := Options{Type: tc.mediaType}
			if tc.inject != "" {
				opts.Extra = json.RawMessage(tc.inject)
			}
			if _, _, err := p.SearchRaw("rain window", 5, opts); err != nil {
				t.Fatalf("SearchRaw() error = %v", err)
			}
			if gotPath != tc.wantPath {
				t.Errorf("path = %q, want %q", gotPath, tc.wantPath)
			}
			for key, want := range tc.wantParams {
				if got := gotQuery.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if tc.absentParam != "" && gotQuery.Get(tc.absentParam) != "" {
				t.Errorf("%s = %q, want empty", tc.absentParam, gotQuery.Get(tc.absentParam))
			}
		})
	}
}

func TestPixabayUnsupportedType(t *testing.T) {
	p := NewPixabayProvider("pb-key")
	if _, _, err := p.SearchRaw("q", 5, Options{Type: "audio"}); err == nil {
		t.Fatal("SearchRaw() error = nil, want unsupported type error")
	}
}

func TestPixabayErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	p := &PixabayProvider{apiKey: "pb-key", baseURL: srv.URL}
	if _, _, err := p.SearchRaw("q", 5, Options{}); err == nil {
		t.Fatal("SearchRaw() error = nil, want API error")
	}
}

func TestPixabayPreviewMasksKey(t *testing.T) {
	p := NewPixabayProvider("pb-key")
	plan, err := p.Preview("rain window", 5, Options{
		Type:  "photo",
		Extra: json.RawMessage(`{"orientation":"horizontal"}`),
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if plan.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", plan.Method)
	}
	u, err := url.Parse(plan.URL)
	if err != nil {
		t.Fatalf("parse preview URL: %v", err)
	}
	values := u.Query()
	if got := values.Get("key"); got == "pb-key" || got == "" {
		t.Errorf("preview key = %q, want a masked non-empty value", got)
	}
	if got := values.Get("image_type"); got != "photo" {
		t.Errorf("image_type = %q, want photo", got)
	}
	if got := values.Get("orientation"); got != "horizontal" {
		t.Errorf("orientation = %q, want horizontal", got)
	}
}

func TestPixabayCachesResponses(t *testing.T) {
	t.Setenv("AIGC_CLI_CACHE_DIR", t.TempDir())

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, `{"hits":[]}`)
	}))
	defer srv.Close()

	p := &PixabayProvider{apiKey: "pb-key", baseURL: srv.URL}
	if _, _, err := p.SearchRaw("rain", 5, Options{}); err != nil {
		t.Fatalf("first SearchRaw() error = %v", err)
	}
	if _, _, err := p.SearchRaw("rain", 5, Options{}); err != nil {
		t.Fatalf("second SearchRaw() error = %v", err)
	}
	if calls != 1 {
		t.Errorf("HTTP calls = %d, want 1 (24h cache)", calls)
	}
	if _, _, err := p.SearchRaw("rain", 5, Options{NoCache: true}); err != nil {
		t.Fatalf("refresh SearchRaw() error = %v", err)
	}
	if calls != 2 {
		t.Errorf("HTTP calls after --refresh = %d, want 2", calls)
	}
}

func TestPixabayCacheKeyExcludesSecret(t *testing.T) {
	a := cacheKey("https://pixabay.com/api/?key=secret-a&q=rain&per_page=5")
	b := cacheKey("https://pixabay.com/api/?key=secret-b&q=rain&per_page=5")
	if a != b {
		t.Error("cacheKey must not depend on the API key")
	}
	if strings.Contains(a, "secret") {
		t.Error("cacheKey leaked the API key")
	}
}

func TestPixabayNormalizedLimitTrimsMinimumPage(t *testing.T) {
	t.Setenv("AIGC_CLI_CACHE_DIR", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"hits":[
			{"pageURL":"https://pixabay.com/a/","tags":"a"},
			{"pageURL":"https://pixabay.com/b/","tags":"b"},
			{"pageURL":"https://pixabay.com/c/","tags":"c"}
		]}`)
	}))
	defer srv.Close()

	p := &PixabayProvider{apiKey: "pb-key", baseURL: srv.URL}
	results, _, err := p.SearchRaw("q", 1, Options{})
	if err != nil {
		t.Fatalf("SearchRaw() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1 (normalized limit)", len(results))
	}
}
