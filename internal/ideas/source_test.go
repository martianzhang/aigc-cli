package ideas

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func sourceNames(sources []Source) []string {
	names := make([]string, 0, len(sources))
	for _, src := range sources {
		names = append(names, src.Name())
	}
	return names
}

func TestResolveSources(t *testing.T) {
	allWithLocal := []string{SourceLocal, SourceAIPromptLibrary, SourcePromptsChat, SourceOpenArt, SourceCivitai}
	allOnline := []string{SourceAIPromptLibrary, SourcePromptsChat, SourceOpenArt, SourceCivitai}

	tests := []struct {
		name     string
		specs    []string
		hasLocal bool
		want     []string
		wantErr  bool
	}{
		{name: "all with local", specs: []string{SourceAll}, hasLocal: true, want: allWithLocal},
		{name: "all without local", specs: []string{SourceAll}, hasLocal: false, want: allOnline},
		{name: "empty specs default to all with local", specs: nil, hasLocal: true, want: allWithLocal},
		{name: "empty specs default to all without local", specs: []string{}, hasLocal: false, want: allOnline},
		{name: "local with local", specs: []string{SourceLocal}, hasLocal: true, want: []string{SourceLocal}},
		{name: "local without local", specs: []string{SourceLocal}, hasLocal: false, wantErr: true},
		{name: "each individual name", specs: []string{SourceAIPromptLibrary}, hasLocal: false, want: []string{SourceAIPromptLibrary}},
		{name: "order preserved", specs: []string{SourceOpenArt, SourceAIPromptLibrary}, hasLocal: false, want: []string{SourceOpenArt, SourceAIPromptLibrary}},
		{name: "comma separated in one element", specs: []string{"aipromptslibrary, prompts.chat"}, hasLocal: false, want: []string{SourceAIPromptLibrary, SourcePromptsChat}},
		{name: "repeatable duplicates deduped", specs: []string{SourceOpenArt, SourceOpenArt}, hasLocal: false, want: []string{SourceOpenArt}},
		{name: "comma separated duplicates deduped", specs: []string{"openart,openart,OPENART"}, hasLocal: false, want: []string{SourceOpenArt}},
		{name: "case insensitive", specs: []string{"OPENART", "Prompts.Chat"}, hasLocal: false, want: []string{SourceOpenArt, SourcePromptsChat}},
		{name: "all alongside other names expands once", specs: []string{SourceOpenArt, SourceAll}, hasLocal: true, want: allWithLocal},
		{name: "whitespace only specs fall back to all", specs: []string{" ", ""}, hasLocal: false, want: allOnline},
		{name: "unknown name", specs: []string{"duckduckgo"}, hasLocal: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveSources(tt.specs, tt.hasLocal)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveSources(%v, %v) = %v, want error", tt.specs, tt.hasLocal, sourceNames(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveSources(%v, %v) unexpected error: %v", tt.specs, tt.hasLocal, err)
			}
			if names := sourceNames(got); !slices.Equal(names, tt.want) {
				t.Fatalf("ResolveSources(%v, %v) = %v, want %v", tt.specs, tt.hasLocal, names, tt.want)
			}
		})
	}
}

func TestResolveSources_unknownNameMessage(t *testing.T) {
	_, err := ResolveSources([]string{"duckduckgo"}, true)
	if err == nil {
		t.Fatal("ResolveSources() = nil error, want unknown source error")
	}
	if !strings.Contains(err.Error(), `unknown source "duckduckgo"`) {
		t.Errorf("error = %q, want it to name the unknown source", err)
	}
	for _, name := range AllSourceNames() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error = %q, want valid source %q listed", err, name)
		}
	}
}

func TestResolveSources_localMissingMessage(t *testing.T) {
	_, err := ResolveSources([]string{SourceLocal}, false)
	if err == nil {
		t.Fatal("ResolveSources(local, false) = nil error, want error")
	}
	if !strings.Contains(err.Error(), "ideas init") {
		t.Errorf("error = %q, want the ideas init hint", err)
	}
}

func TestHasLocalAndOnlineSources(t *testing.T) {
	all, err := ResolveSources([]string{SourceAll}, true)
	if err != nil {
		t.Fatalf("ResolveSources(all, true) unexpected error: %v", err)
	}
	if !HasLocal(all) {
		t.Error("HasLocal(all with local) = false, want true")
	}
	if names := sourceNames(OnlineSources(all)); !slices.Equal(names, OnlineSourceNames()) {
		t.Errorf("OnlineSources(all) = %v, want %v", names, OnlineSourceNames())
	}

	online, err := ResolveSources([]string{SourceAll}, false)
	if err != nil {
		t.Fatalf("ResolveSources(all, false) unexpected error: %v", err)
	}
	if HasLocal(online) {
		t.Error("HasLocal(all without local) = true, want false")
	}
	if names := sourceNames(OnlineSources(online)); !slices.Equal(names, OnlineSourceNames()) {
		t.Errorf("OnlineSources(all online) = %v, want %v", names, OnlineSourceNames())
	}
}

func TestLocalSourceSearch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	dir := filepath.Join(home, ".config", "aigc-cli", "ideas")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("cannot create ideas dir: %v", err)
	}
	entries := []IdeaEntry{
		{Title: "cat photo", Prompt: "a cute cat sitting on a mat", Lang: "en"},
		{Title: "dog photo", Prompt: "a happy dog running in the park", Lang: "en"},
	}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("cannot marshal fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ideas.json"), data, 0o644); err != nil {
		t.Fatalf("cannot write fixture: %v", err)
	}

	src := localSource{}
	if src.Name() != SourceLocal {
		t.Fatalf("Name() = %q, want %q", src.Name(), SourceLocal)
	}

	got, err := src.Search(context.Background(), "cat", 0)
	if err != nil {
		t.Fatalf("Search(cat) unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Title != "cat photo" {
		t.Fatalf("Search(cat) = %+v, want the cat entry", got)
	}

	got, err = src.Search(context.Background(), "photo", 1)
	if err != nil {
		t.Fatalf("Search(photo) unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Search(photo, limit=1) returned %d entries, want 1", len(got))
	}

	if _, err := src.Search(context.Background(), "   ", 0); !errors.Is(err, errEmptyQuery) {
		t.Fatalf("Search(blank) error = %v, want errEmptyQuery", err)
	}
}
