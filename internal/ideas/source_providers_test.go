package ideas

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestAIPromptLibrarySource(t *testing.T) {
	body := `{"items":[
		{"slug":"cyberpunk-city","title":"Cyberpunk City","summary":"Neon skyline","url":"https://aipromptslibrary.sh/prompts/cyberpunk-city","category":"image_generation","audience":"","tags":[],"classification":"","functional":"","quality":"","safety":"","prompt":"A neon cyberpunk city at night"},
		{"slug":"neon-alley","title":"Neon Alley","url":"https://aipromptslibrary.sh/prompts/neon-alley","prompt":"{\"subject\":\"alley\"}"}
	]}`
	var path string
	var query url.Values
	srv := newFixtureServer(body, &path, &query)
	defer srv.Close()
	defer overrideBaseURL(&aipromptLibraryBaseURL, srv.URL)()

	src := aiPromptLibrarySource{}
	if src.Name() != SourceAIPromptLibrary {
		t.Fatalf("Name() = %q, want %q", src.Name(), SourceAIPromptLibrary)
	}

	got, err := src.Search(context.Background(), "cyberpunk city", 5)
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}
	if path != "/api/prompts" {
		t.Errorf("request path = %q, want /api/prompts", path)
	}
	if got := query.Get("q"); got != "cyberpunk city" {
		t.Errorf("q = %q, want cyberpunk city", got)
	}
	if got := query.Get("category"); got != "image_generation" {
		t.Errorf("category = %q, want image_generation", got)
	}
	if got := query.Get("include_prompt_text"); got != "true" {
		t.Errorf("include_prompt_text = %q, want true", got)
	}
	if got := query.Get("pageSize"); got != "5" {
		t.Errorf("pageSize = %q, want 5", got)
	}

	if len(got) != 2 {
		t.Fatalf("Search() returned %d entries, want 2", len(got))
	}
	if got[0].Title != "Cyberpunk City" || got[0].Prompt != "A neon cyberpunk city at night" ||
		got[0].SourceURL != "https://aipromptslibrary.sh/prompts/cyberpunk-city" || got[0].Lang != "en" {
		t.Errorf("entry[0] = %+v, want the cyberpunk mapping", got[0])
	}
	if got[1].Prompt != `{"subject":"alley"}` {
		t.Errorf("entry[1].Prompt = %q, want the verbatim JSON string", got[1].Prompt)
	}

	limited, err := src.Search(context.Background(), "cyberpunk city", 1)
	if err != nil {
		t.Fatalf("Search(limit=1) unexpected error: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("Search(limit=1) returned %d entries, want 1", len(limited))
	}

	if _, err := src.Search(context.Background(), "  ", 5); !errors.Is(err, errEmptyQuery) {
		t.Fatalf("Search(blank) error = %v, want errEmptyQuery", err)
	}
}

func TestPromptsChatSource(t *testing.T) {
	body := `{"prompts":[
		{"title":"Portrait Pro","slug":"portrait-pro","description":"desc","content":"Studio portrait prompt","type":"text","mediaUrl":"https://prompts.chat/media/portrait.png","author":{"name":"Jane"}},
		{"title":"No Media","slug":"no-media","content":"Another prompt","author":{"name":""}}
	]}`
	var path string
	var query url.Values
	srv := newFixtureServer(body, &path, &query)
	defer srv.Close()
	defer overrideBaseURL(&promptsChatBaseURL, srv.URL)()

	src := promptsChatSource{}
	if src.Name() != SourcePromptsChat {
		t.Fatalf("Name() = %q, want %q", src.Name(), SourcePromptsChat)
	}

	got, err := src.Search(context.Background(), "portrait", 5)
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}
	if path != "/api/prompts" {
		t.Errorf("request path = %q, want /api/prompts", path)
	}
	if got := query.Get("q"); got != "portrait" {
		t.Errorf("q = %q, want portrait", got)
	}
	if got := query.Get("perPage"); got != "5" {
		t.Errorf("perPage = %q, want 5", got)
	}

	if len(got) != 2 {
		t.Fatalf("Search() returned %d entries, want 2", len(got))
	}
	first := got[0]
	if first.Title != "Portrait Pro" || first.Prompt != "Studio portrait prompt" || first.Author != "Jane" || first.Lang != "en" {
		t.Errorf("entry[0] = %+v, want the portrait mapping", first)
	}
	if want := srv.URL + "/prompts/portrait-pro"; first.SourceURL != want {
		t.Errorf("entry[0].SourceURL = %q, want %q", first.SourceURL, want)
	}
	if !slices.Equal(first.ImageURLs, []string{"https://prompts.chat/media/portrait.png"}) {
		t.Errorf("entry[0].ImageURLs = %v, want the mediaUrl", first.ImageURLs)
	}
	if got[1].SourceURL != srv.URL+"/prompts/no-media" {
		t.Errorf("entry[1].SourceURL = %q, want the canonical slug URL", got[1].SourceURL)
	}
	if len(got[1].ImageURLs) != 0 {
		t.Errorf("entry[1].ImageURLs = %v, want empty", got[1].ImageURLs)
	}

	if _, err := src.Search(context.Background(), "\n\t", 5); !errors.Is(err, errEmptyQuery) {
		t.Fatalf("Search(blank) error = %v, want errEmptyQuery", err)
	}
}

func TestOpenArtSource(t *testing.T) {
	longPrompt := strings.Repeat("x", 70) + "\nsecond line"
	jsonPrompt := strings.ReplaceAll(longPrompt, "\n", `\n`)
	body := `{"items":[
		{"id":"1","prompt":"` + jsonPrompt + `","image_url":"https://openart.ai/img/1.png","ai_model":"m","userProfile":{"name":"Bob"}},
		{"id":"2","prompt":"","image_url":"https://openart.ai/img/2.png","ai_model":"m","userProfile":{"name":"Empty"}},
		{"id":"3","prompt":"short prompt","image_url":"","ai_model":"m","userProfile":{}}
	]}`
	var path string
	var query url.Values
	srv := newFixtureServer(body, &path, &query)
	defer srv.Close()
	defer overrideBaseURL(&openArtBaseURL, srv.URL)()

	src := openArtSource{}
	if src.Name() != SourceOpenArt {
		t.Fatalf("Name() = %q, want %q", src.Name(), SourceOpenArt)
	}

	got, err := src.Search(context.Background(), "portrait", 5)
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}
	if path != "/api/search" {
		t.Errorf("request path = %q, want /api/search", path)
	}
	if query.Encode() != "q=portrait" {
		t.Errorf("query = %q, want only q=portrait", query.Encode())
	}

	if len(got) != 2 {
		t.Fatalf("Search() returned %d entries, want 2 (empty-prompt item skipped)", len(got))
	}
	first := got[0]
	if first.SourceURL != "" {
		t.Errorf("entry[0].SourceURL = %q, want empty (no canonical URL exists)", first.SourceURL)
	}
	if want := strings.Repeat("x", openArtTitleMaxRunes) + "…"; first.Title != want {
		t.Errorf("entry[0].Title = %q, want %q", first.Title, want)
	}
	if first.Prompt != longPrompt {
		t.Errorf("entry[0].Prompt = %q, want the full prompt", first.Prompt)
	}
	if first.Author != "Bob" || first.Lang != "en" {
		t.Errorf("entry[0] = %+v, want author Bob and lang en", first)
	}
	if !slices.Equal(first.ImageURLs, []string{"https://openart.ai/img/1.png"}) {
		t.Errorf("entry[0].ImageURLs = %v, want the image_url", first.ImageURLs)
	}
	if got[1].Prompt != "short prompt" {
		t.Errorf("entry[1].Prompt = %q, want short prompt", got[1].Prompt)
	}
	if len(got[1].ImageURLs) != 0 {
		t.Errorf("entry[1].ImageURLs = %v, want empty", got[1].ImageURLs)
	}

	limited, err := src.Search(context.Background(), "portrait", 1)
	if err != nil {
		t.Fatalf("Search(limit=1) unexpected error: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("Search(limit=1) returned %d entries, want 1 (client-side truncation)", len(limited))
	}

	if _, err := src.Search(context.Background(), "", 5); !errors.Is(err, errEmptyQuery) {
		t.Fatalf("Search(blank) error = %v, want errEmptyQuery", err)
	}
}

func TestTitleFromPrompt(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{name: "single line", prompt: "  a short prompt  ", want: "a short prompt"},
		{name: "first line only", prompt: "first line\nsecond line", want: "first line"},
		{name: "empty", prompt: "", want: ""},
		{name: "exactly max runes", prompt: strings.Repeat("a", openArtTitleMaxRunes), want: strings.Repeat("a", openArtTitleMaxRunes)},
		{name: "truncated ascii", prompt: strings.Repeat("a", openArtTitleMaxRunes+1), want: strings.Repeat("a", openArtTitleMaxRunes) + "…"},
		{name: "truncated cjk by runes", prompt: strings.Repeat("你", openArtTitleMaxRunes+5), want: strings.Repeat("你", openArtTitleMaxRunes) + "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := titleFromPrompt(tt.prompt); got != tt.want {
				t.Fatalf("titleFromPrompt(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}
