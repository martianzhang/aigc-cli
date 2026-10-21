package webfetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
)

func allowAll(string) error { return nil }

func TestFetchConvertsHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Test Page</title></head><body><article><h1>Hello</h1><p>Readable body text that is long enough for readability.</p></article></body></html>`))
	}))
	defer srv.Close()

	doc, err := Fetch(context.Background(), srv.URL, Options{ValidateURL: allowAll})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !doc.Converted {
		t.Error("Converted = false, want true for HTML")
	}
	if doc.Title != "Test Page" {
		t.Errorf("Title = %q, want Test Page", doc.Title)
	}
	if !strings.Contains(doc.Content, "Hello") || !strings.Contains(doc.Content, "Readable body") {
		t.Errorf("Content = %q, want extracted article", doc.Content)
	}
	if doc.RawHTML == "" {
		t.Error("RawHTML is empty, want the decoded HTML")
	}
	if doc.FinalURL != srv.URL {
		t.Errorf("FinalURL = %q, want %q", doc.FinalURL, srv.URL)
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, Options{ValidateURL: allowAll})
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("error = %v, want HTTP 404", err)
	}
}

func TestFetchRejectsBlockedURL(t *testing.T) {
	_, err := Fetch(context.Background(), "http://127.0.0.1/secret", Options{
		ValidateURL: func(string) error { return errors.New("non-public address") },
	})
	if err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("error = %v, want the validator rejection", err)
	}
}

func TestFetchDecodesGBK(t *testing.T) {
	html := `<html><head><title>中文标题</title></head><body><article><p>你好世界，这是一个 GBK 页面。</p></article></body></html>`
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(html))
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=gbk")
		_, _ = w.Write(encoded)
	}))
	defer srv.Close()

	doc, err := Fetch(context.Background(), srv.URL, Options{ValidateURL: allowAll})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(doc.Content, "你好世界") || doc.Title != "中文标题" {
		t.Errorf("content/title = %q/%q, want decoded Chinese text", doc.Content, doc.Title)
	}
}

func TestFetchReturnsTextVerbatim(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	}))
	defer srv.Close()

	doc, err := Fetch(context.Background(), srv.URL, Options{ValidateURL: allowAll})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if doc.Converted {
		t.Error("Converted = true, want false for JSON")
	}
	if doc.Content != `{"hello":"world"}` {
		t.Errorf("Content = %q, want verbatim JSON", doc.Content)
	}
}

func TestFetchRejectsBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7"))
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, Options{ValidateURL: allowAll})
	if err == nil || !strings.Contains(err.Error(), "unsupported content type") {
		t.Fatalf("error = %v, want unsupported content type", err)
	}
}

func TestFetchAppliesHeaders(t *testing.T) {
	var userAgent, custom, accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userAgent = r.Header.Get("User-Agent")
		custom = r.Header.Get("X-Test")
		accept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL, Options{
		ValidateURL: allowAll,
		Headers: []Header{
			{Name: "User-Agent", Value: "custom-agent"},
			{Name: "X-Test", Value: "1"},
			{Name: "Accept", Value: ""},
		},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if userAgent != "custom-agent" || custom != "1" {
		t.Errorf("headers = UA %q, X-Test %q, want custom values", userAgent, custom)
	}
	if accept != "" {
		t.Errorf("Accept = %q, want removed", accept)
	}
}

func TestFetchTruncatesOversizedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(strings.Repeat("a", 100)))
	}))
	defer srv.Close()

	doc, err := Fetch(context.Background(), srv.URL, Options{MaxBytes: 10, ValidateURL: allowAll})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !doc.Truncated {
		t.Error("Truncated = false, want true")
	}
	if doc.Bytes != 10 || len(doc.Content) != 10 {
		t.Errorf("Bytes/Content = %d/%d, want 10/10", doc.Bytes, len(doc.Content))
	}
}

func TestIsHTMLBody(t *testing.T) {
	tests := []struct {
		mediaType string
		body      string
		want      bool
	}{
		{mediaType: "text/html", body: "<html></html>", want: true},
		{mediaType: "application/xhtml+xml", body: "<html/>", want: true},
		{mediaType: "application/json", body: `{"a":1}`, want: false},
		{mediaType: "text/plain", body: "hello", want: false},
		{mediaType: "application/xml", body: "<rss></rss>", want: false},
		{mediaType: "", body: "<!doctype html><html>", want: true},
		{mediaType: "", body: `<html lang="en">`, want: true},
		{mediaType: "", body: `{"a":1}`, want: false},
		{mediaType: "", body: "", want: false},
	}
	for _, tc := range tests {
		isHTML := tc.mediaType == "text/html" || tc.mediaType == "application/xhtml+xml" ||
			(tc.mediaType == "" && looksLikeHTML([]byte(tc.body)))
		if isHTML != tc.want {
			t.Errorf("isHTML(%q, %q) = %v, want %v", tc.mediaType, tc.body, isHTML, tc.want)
		}
	}
}
