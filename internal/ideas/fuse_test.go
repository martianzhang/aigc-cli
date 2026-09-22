package ideas

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSource struct {
	name     string
	entries  []IdeaEntry
	err      error
	gotQuery string
	gotLimit int
}

func (f *fakeSource) Name() string { return f.name }

func (f *fakeSource) Search(_ context.Context, query string, limit int) ([]IdeaEntry, error) {
	f.gotQuery = query
	f.gotLimit = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

func TestSearchOnline(t *testing.T) {
	t.Run("round robin with one failing source", func(t *testing.T) {
		goodA := &fakeSource{name: "good-a", entries: []IdeaEntry{{Title: "a1", Prompt: "pa1"}, {Title: "a2", Prompt: "pa2"}, {Title: "a3", Prompt: "pa3"}}}
		goodB := &fakeSource{name: "good-b", entries: []IdeaEntry{{Title: "b1", Prompt: "pb1"}, {Title: "b2", Prompt: "pb2"}}}
		bad := &fakeSource{name: "bad", err: errors.New("boom")}

		got, errs := SearchOnline(context.Background(), []Source{goodA, goodB, bad}, "cyberpunk", 5)
		if len(errs) != 1 {
			t.Fatalf("SearchOnline() returned %d errors, want 1: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), "source bad") || !strings.Contains(errs[0].Error(), "boom") {
			t.Errorf("error = %q, want it to name the source and cause", errs[0])
		}

		var titles []string
		for _, entry := range got {
			titles = append(titles, entry.Title)
		}
		if want := []string{"a1", "b1", "a2", "b2", "a3"}; strings.Join(titles, ",") != strings.Join(want, ",") {
			t.Fatalf("round robin order = %v, want %v", titles, want)
		}
		if goodA.gotQuery != "cyberpunk" || goodA.gotLimit != 5 {
			t.Errorf("source received query=%q limit=%d, want cyberpunk/5", goodA.gotQuery, goodA.gotLimit)
		}
	})

	t.Run("dedupes entries shared across sources", func(t *testing.T) {
		shared := IdeaEntry{Title: "shared", Prompt: "shared prompt", SourceURL: "https://example.com/shared"}
		goodA := &fakeSource{name: "good-a", entries: []IdeaEntry{shared, {Title: "a2", Prompt: "pa2"}}}
		goodB := &fakeSource{name: "good-b", entries: []IdeaEntry{shared}}

		got, errs := SearchOnline(context.Background(), []Source{goodA, goodB}, "q", 10)
		if len(errs) != 0 {
			t.Fatalf("SearchOnline() errors = %v, want none", errs)
		}
		if len(got) != 2 || got[0].Title != "shared" || got[1].Title != "a2" {
			t.Fatalf("SearchOnline() = %+v, want the shared entry once then a2", got)
		}
	})

	t.Run("all sources fail", func(t *testing.T) {
		one := &fakeSource{name: "one", err: errors.New("first")}
		two := &fakeSource{name: "two", err: errors.New("second")}

		got, errs := SearchOnline(context.Background(), []Source{one, two}, "q", 10)
		if len(got) != 0 {
			t.Fatalf("SearchOnline() = %+v, want no entries", got)
		}
		if len(errs) != 2 {
			t.Fatalf("SearchOnline() returned %d errors, want 2", len(errs))
		}
	})
}

func TestFuseRRF(t *testing.T) {
	t.Run("fuses overlapping and unique entries", func(t *testing.T) {
		shared := IdeaEntry{Title: "shared", Prompt: "p", SourceURL: "https://example.com/a"}
		listA := []IdeaEntry{shared, {Title: "a2", Prompt: "pa2"}}
		listB := []IdeaEntry{shared, {Title: "b2", Prompt: "pb2"}}

		got := FuseRRF([][]IdeaEntry{listA, listB}, 0)
		if len(got) != 3 {
			t.Fatalf("FuseRRF() returned %d results, want 3", len(got))
		}
		if got[0].Entry.Title != "shared" {
			t.Fatalf("results[0] = %q, want the entry ranked first in both lists", got[0].Entry.Title)
		}
		rrf := 2.0 / (rrfK + 1)
		if want := int(rrf * 1000); got[0].Score != want {
			t.Errorf("results[0].Score = %d, want %d", got[0].Score, want)
		}
		if got[0].Score <= got[1].Score || got[1].Score != got[2].Score {
			t.Errorf("scores = %d,%d,%d, want first place strictly higher and the rest equal", got[0].Score, got[1].Score, got[2].Score)
		}
	})

	t.Run("dedupes by prompt when source url is empty", func(t *testing.T) {
		dupA := IdeaEntry{Title: "first", Prompt: "same prompt"}
		dupB := IdeaEntry{Title: "second", Prompt: "same prompt"}
		got := FuseRRF([][]IdeaEntry{{dupA}, {dupB}}, 0)
		if len(got) != 1 {
			t.Fatalf("FuseRRF() returned %d results, want 1", len(got))
		}
		if got[0].Entry.Title != "first" {
			t.Errorf("kept entry = %q, want the first seen", got[0].Entry.Title)
		}
	})

	t.Run("respects limit", func(t *testing.T) {
		list := []IdeaEntry{{Title: "a", Prompt: "pa"}, {Title: "b", Prompt: "pb"}, {Title: "c", Prompt: "pc"}}
		got := FuseRRF([][]IdeaEntry{list}, 2)
		if len(got) != 2 || got[0].Entry.Title != "a" || got[1].Entry.Title != "b" {
			t.Fatalf("FuseRRF(limit=2) = %+v, want a and b", got)
		}
	})

	t.Run("deterministic tie break by first appearance", func(t *testing.T) {
		listA := []IdeaEntry{{Title: "a1", Prompt: "pa1"}, {Title: "a2", Prompt: "pa2"}}
		listB := []IdeaEntry{{Title: "b1", Prompt: "pb1"}, {Title: "b2", Prompt: "pb2"}}
		got := FuseRRF([][]IdeaEntry{listA, listB}, 0)
		var titles []string
		for _, result := range got {
			titles = append(titles, result.Entry.Title)
		}
		if want := "a1,b1,a2,b2"; strings.Join(titles, ",") != want {
			t.Fatalf("order = %v, want %s (first-seen wins ties)", titles, want)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		if got := FuseRRF(nil, 5); len(got) != 0 {
			t.Fatalf("FuseRRF(nil) = %+v, want empty", got)
		}
	})

	t.Run("single list keeps rank order", func(t *testing.T) {
		list := []IdeaEntry{{Title: "a", Prompt: "pa"}, {Title: "b", Prompt: "pb"}}
		got := FuseRRF([][]IdeaEntry{list}, 0)
		if len(got) != 2 || got[0].Entry.Title != "a" || got[1].Entry.Title != "b" {
			t.Fatalf("FuseRRF(single list) = %+v, want rank order preserved", got)
		}
		if got[0].Score <= got[1].Score {
			t.Errorf("scores = %d,%d, want rank 1 above rank 2", got[0].Score, got[1].Score)
		}
	})
}

func TestMergeRoundRobin(t *testing.T) {
	got := mergeRoundRobin([][]IdeaEntry{
		{{Title: "a1", Prompt: "pa1"}, {Title: "a2", Prompt: "pa2"}},
		{{Title: "b1", Prompt: "pb1"}},
		{},
	})
	var titles []string
	for _, entry := range got {
		titles = append(titles, entry.Title)
	}
	if want := "a1,b1,a2"; strings.Join(titles, ",") != want {
		t.Fatalf("mergeRoundRobin() = %v, want %s", titles, want)
	}
	if empty := mergeRoundRobin(nil); len(empty) != 0 {
		t.Fatalf("mergeRoundRobin(nil) = %v, want empty", empty)
	}
}
