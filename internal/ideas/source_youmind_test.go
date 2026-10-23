package ideas

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestYouMindSourceSearch(t *testing.T) {
	type captured struct {
		path, origin, referer, query, model string
		limit                               int
	}
	var got []captured
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Q     string `json:"q"`
			Model string `json:"model"`
			Limit int    `json:"limit"`
		}
		_ = json.Unmarshal(body, &payload)
		mu.Lock()
		got = append(got, captured{
			path:    r.URL.Path,
			origin:  r.Header.Get("Origin"),
			referer: r.Header.Get("Referer"),
			query:   payload.Q,
			model:   payload.Model,
			limit:   payload.Limit,
		})
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/youmarketing-api/video-prompts":
			io.WriteString(w, `{"prompts":[
				{"id":2,"title":"Video A","slug":"video-a","content":"video prompt A","language":"en","author":{"name":"va"},"videos":[{"thumbnail":"https://img/v.png"}]}
			]}`)
		case payload.Model == "gpt-image-2":
			io.WriteString(w, `{"prompts":[
				{"id":1,"title":"Image A","slug":"image-a","content":"image prompt A","language":"en","author":{"name":"ia"},"media":["https://img/a.jpg"]},
				{"id":3,"title":"Image C","slug":"image-c","content":"image prompt C","language":"en","media":["https://img/c.jpg"]}
			]}`)
		default:
			io.WriteString(w, `{"prompts":[]}`)
		}
	}))
	defer srv.Close()
	prev := youmindBaseURL
	youmindBaseURL = srv.URL
	t.Cleanup(func() { youmindBaseURL = prev })

	entries, err := youmindSource{}.Search(context.Background(), "cyberpunk", 3)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3: %+v", len(entries), entries)
	}
	// Round-robin: image A, video A, image C.
	if entries[0].Title != "Image A" || entries[1].Title != "Video A" || entries[2].Title != "Image C" {
		t.Errorf("order = %v, want image A, video A, image C", titles(entries))
	}
	if entries[0].Prompt != "image prompt A" || entries[0].Author != "ia" {
		t.Errorf("image entry = %+v", entries[0])
	}
	if entries[0].SourceURL != srv.URL+"/prompts/image-a" {
		t.Errorf("image URL = %q, want /prompts/image-a", entries[0].SourceURL)
	}
	if len(entries[0].ImageURLs) != 1 || entries[0].ImageURLs[0] != "https://img/a.jpg" {
		t.Errorf("image URLs = %v", entries[0].ImageURLs)
	}
	if entries[1].SourceURL != srv.URL+"/video-prompts/video-a" {
		t.Errorf("video URL = %q, want /video-prompts/video-a", entries[1].SourceURL)
	}
	if len(entries[1].ImageURLs) != 1 || entries[1].ImageURLs[0] != "https://img/v.png" {
		t.Errorf("video thumbnails = %v", entries[1].ImageURLs)
	}

	if len(got) != len(youmindQueries) {
		t.Fatalf("requests = %d, want %d", len(got), len(youmindQueries))
	}
	for _, req := range got {
		if req.origin != srv.URL || req.referer != srv.URL+"/prompts" {
			t.Errorf("headers = origin %q referer %q, want site origin/referer", req.origin, req.referer)
		}
		if req.query != "cyberpunk" || req.limit != 3 {
			t.Errorf("payload = q %q limit %d, want cyberpunk/3", req.query, req.limit)
		}
	}
}

func TestYouMindSourceEmptyQuery(t *testing.T) {
	if _, err := (youmindSource{}).Search(context.Background(), "  ", 5); err == nil {
		t.Fatal("Search() error = nil, want empty-query error")
	}
}

func TestYouMindSourcePartialFailureKeepsResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/youmarketing-api/video-prompts" {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, `{"error":"video down"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"prompts":[{"id":1,"title":"A","slug":"a","content":"prompt A","language":"en"}]}`)
	}))
	defer srv.Close()
	prev := youmindBaseURL
	youmindBaseURL = srv.URL
	t.Cleanup(func() { youmindBaseURL = prev })

	entries, err := youmindSource{}.Search(context.Background(), "portrait", 5)
	if err != nil {
		t.Fatalf("Search() error = %v, want nil while one endpoint works", err)
	}
	if len(entries) == 0 {
		t.Fatal("Search() returned no entries, want results from healthy endpoints")
	}
}

func TestYouMindSourceAllFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"error":"Forbidden"}`)
	}))
	defer srv.Close()
	prev := youmindBaseURL
	youmindBaseURL = srv.URL
	t.Cleanup(func() { youmindBaseURL = prev })

	_, err := youmindSource{}.Search(context.Background(), "portrait", 5)
	if err == nil {
		t.Fatal("Search() error = nil, want API error")
	}
	if !strings.Contains(err.Error(), "Forbidden") {
		t.Errorf("error = %q, want vendor message", err)
	}
}

func TestYouMindSourceFallsBackToTranslatedContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"prompts":[{"id":1,"title":"T","slug":"t","translatedContent":"translated prompt","language":"zh"}]}`)
	}))
	defer srv.Close()
	prev := youmindBaseURL
	youmindBaseURL = srv.URL
	t.Cleanup(func() { youmindBaseURL = prev })

	entries, err := youmindSource{}.Search(context.Background(), "portrait", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(entries) == 0 || entries[0].Prompt != "translated prompt" {
		t.Errorf("entries = %+v, want translated content fallback", entries)
	}
}

func TestInterleaveEntriesDedupes(t *testing.T) {
	lists := [][]IdeaEntry{
		{{Title: "a", SourceURL: "u1"}, {Title: "b", SourceURL: "u2"}},
		{{Title: "a", SourceURL: "u1"}, {Title: "c", SourceURL: "u3"}},
	}
	got := interleaveEntries(lists, 0)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %v", len(got), titles(got))
	}
	if got[0].Title != "a" || got[1].Title != "b" || got[2].Title != "c" {
		t.Errorf("dedupe order = %v, want a,b,c", titles(got))
	}
}

func titles(entries []IdeaEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Title
	}
	return out
}
