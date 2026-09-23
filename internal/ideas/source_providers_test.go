package ideas

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
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

func TestCivitaiSource(t *testing.T) {
	var modelsPath, imagesPath string
	var modelsQuery, imagesQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/models":
			modelsPath, modelsQuery = r.URL.Path, r.URL.Query()
			_, _ = w.Write([]byte(`{"items":[
				{"id":1,"modelVersions":[{"id":111},{"id":110}]}
			]}`))
		case "/api/v1/images":
			imagesPath, imagesQuery = r.URL.Path, r.URL.Query()
			_, _ = w.Write([]byte(`{"items":[
				{"id":9001,"url":"https://img.civitai.com/1.jpeg","username":"Yoruuu","meta":{"prompt":"A neon cyberpunk city at night","negativePrompt":"blur"}},
				{"id":9002,"url":"https://img.civitai.com/2.jpeg","username":"Comfy","meta":{"comfy":"workflow only"}},
				{"id":9003,"url":"https://img.civitai.com/3.jpeg","username":"NoMeta"},
				{"id":9004,"url":"https://img.civitai.com/4.jpeg","username":"Blank","meta":{"prompt":"   "}},
				{"id":9005,"url":"https://img.civitai.com/5.jpeg","username":"Bob","meta":{"prompt":"Second prompt"}}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	defer overrideBaseURL(&civitaiBaseURL, srv.URL)()

	src := civitaiSource{}
	if src.Name() != SourceCivitai {
		t.Fatalf("Name() = %q, want %q", src.Name(), SourceCivitai)
	}

	got, err := src.Search(context.Background(), "cyberpunk", 5)
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}
	if modelsPath != "/api/v1/models" {
		t.Errorf("models path = %q, want /api/v1/models", modelsPath)
	}
	if v := modelsQuery.Get("query"); v != "cyberpunk" {
		t.Errorf("models query = %q, want cyberpunk", v)
	}
	if v := modelsQuery.Get("limit"); v != "3" {
		t.Errorf("models limit = %q, want 3", v)
	}
	if imagesPath != "/api/v1/images" {
		t.Errorf("images path = %q, want /api/v1/images", imagesPath)
	}
	if v := imagesQuery.Get("modelVersionId"); v != "111" {
		t.Errorf("modelVersionId = %q, want 111 (latest version)", v)
	}
	if v := imagesQuery.Get("withMeta"); v != "true" {
		t.Errorf("withMeta = %q, want true", v)
	}
	if v := imagesQuery.Get("nsfw"); v != "None" {
		t.Errorf("nsfw = %q, want None", v)
	}
	if v := imagesQuery.Get("sort"); v != civitaiSort {
		t.Errorf("sort = %q, want %q", v, civitaiSort)
	}
	if v := imagesQuery.Get("limit"); v != "5" {
		t.Errorf("images limit = %q, want 5", v)
	}

	if len(got) != 2 {
		t.Fatalf("Search() returned %d entries, want 2 (meta-less items skipped)", len(got))
	}
	first := got[0]
	if first.Prompt != "A neon cyberpunk city at night" || first.Author != "Yoruuu" || first.Lang != "en" {
		t.Errorf("entry[0] = %+v, want the prompt/author mapping", first)
	}
	if want := "A neon cyberpunk city at night"; first.Title != want {
		t.Errorf("entry[0].Title = %q, want %q", first.Title, want)
	}
	if want := srv.URL + "/images/9001"; first.SourceURL != want {
		t.Errorf("entry[0].SourceURL = %q, want %q", first.SourceURL, want)
	}
	if !slices.Equal(first.ImageURLs, []string{"https://img.civitai.com/1.jpeg"}) {
		t.Errorf("entry[0].ImageURLs = %v, want the image url", first.ImageURLs)
	}
	if got[1].Prompt != "Second prompt" {
		t.Errorf("entry[1].Prompt = %q, want Second prompt", got[1].Prompt)
	}

	limited, err := src.Search(context.Background(), "cyberpunk", 1)
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

func TestCivitaiSourceQueriesEveryModelAndClampsLimit(t *testing.T) {
	var mu sync.Mutex
	var versionIDs []string
	var imageLimit string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/models":
			_, _ = w.Write([]byte(`{"items":[
				{"id":1,"modelVersions":[{"id":111}]},
				{"id":2,"modelVersions":[{"id":222}]},
				{"id":3,"modelVersions":[]}
			]}`))
		case "/api/v1/images":
			mu.Lock()
			versionIDs = append(versionIDs, r.URL.Query().Get("modelVersionId"))
			imageLimit = r.URL.Query().Get("limit")
			mu.Unlock()
			_, _ = w.Write([]byte(`{"items":[{"id":1,"url":"u","username":"a","meta":{"prompt":"p"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	defer overrideBaseURL(&civitaiBaseURL, srv.URL)()

	got, err := civitaiSource{}.Search(context.Background(), "anything", 50)
	if err != nil {
		t.Fatalf("Search() unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	slices.Sort(versionIDs)
	if !slices.Equal(versionIDs, []string{"111", "222"}) {
		t.Errorf("queried model versions = %v, want [111 222] (versionless model skipped)", versionIDs)
	}
	if imageLimit != "12" {
		t.Errorf("images limit = %q, want 12 (clamped by civitaiImageLimit)", imageLimit)
	}
	if len(got) != 1 {
		t.Fatalf("Search() returned %d entries, want 1 (identical images deduped)", len(got))
	}
}

func TestCivitaiSourceReportsErrorWhenAllRequestsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[{"id":1,"modelVersions":[{"id":111}]}]}`))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	defer overrideBaseURL(&civitaiBaseURL, srv.URL)()

	if _, err := (civitaiSource{}).Search(context.Background(), "x", 5); err == nil {
		t.Fatal("Search() = nil error, want the upstream failure surfaced when nothing was collected")
	}
}
