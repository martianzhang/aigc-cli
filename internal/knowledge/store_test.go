package knowledge

import (
	"fmt"
	"reflect"
	"testing"
)

// This file tests the pure (no SQLite) query-building and helper functions
// used by Store. Behavior matches the actual implementation:
//   - ParseSearchQuery strips double quotes via stripFTSReserved.
//   - simplifyFTSQuery joins remaining terms with a single space.
//   - orFTSQuery joins terms with " OR " and returns "" when fewer than two
//     terms of at least three characters remain.

func TestParseSearchQueryCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple term", "hello", "hello"},
		{"short term gets prefix", "cat", "cat*"},
		{"three-char term gets prefix", "abc", "abc*"},
		{"four-char term no prefix", "abcd", "abcd"},
		{"multi word", "term1 term2", "term1 term2"},
		{"short plus long word", "ai knowledgebase", "ai* knowledgebase"},
		{"quoted phrase quotes stripped", `"hello world"`, "hello world"},
		{"quoted short phrase gets prefix", `"a b"`, "a b*"},
		{"quoted phrase with hyphen stays quoted", `"aigc-cli guide"`, `"aigc-cli guide"`},
		{"hyphenated term is quoted", "aigc-cli", `"aigc-cli"`},
		{"dotted term is quoted", "file.md", `"file.md"`},
		{"hyphen inside quoted phrase preserved", `"hello-world test"`, `"hello-world test"`},
		{"mixed quoted and bare", `"hello world" cat`, "hello world cat*"},
		{"empty query", "", ""},
		{"reserved-only falls back to raw", "*", "*"},
		{"whitespace-only falls back to raw", "   ", "   "},
		{"parentheses stripped", "hello (world)", "hello world"},
		{"plus only term", "c++", "c*"},
		{"plus inside term splits and prefixes", "a+b", "a b*"},
		{"stray star dropped", "hello *", "hello"},
		{"unterminated quote", `"hello world`, "hello world"},
		{"newline separated", "hello\nworld", "hello world"},
		{"tab separated", "hello\tworld", "hello world"},
		{"extra spaces collapsed", "hello   world", "hello world"},
		{"cjk term", "知识库", "知识库"},
		{"short ascii plus cjk", "ai 知识库", "ai* 知识库"},
		{"case preserved", "Hello World", "Hello World"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sq := ParseSearchQuery(tt.input)
			if sq.Processed != tt.want {
				t.Errorf("ParseSearchQuery(%q).Processed = %q, want %q", tt.input, sq.Processed, tt.want)
			}
			if sq.Raw != tt.input {
				t.Errorf("ParseSearchQuery(%q).Raw = %q, want raw input preserved", tt.input, sq.Raw)
			}
		})
	}
}

func TestTokenizeFTSCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"empty", "", nil},
		{"single word", "hello", []string{"hello"}},
		{"two words", "hello world", []string{"hello", "world"}},
		{"splits on tabs and newlines", "a\tb\nc", []string{"a", "b", "c"}},
		{"quoted phrase kept whole", `"hello world"`, []string{`"hello world"`}},
		{"quoted plus bare word", `"a b" c`, []string{`"a b"`, "c"}},
		{"two quoted phrases", `"a b" "c d"`, []string{`"a b"`, `"c d"`}},
		{"unterminated quote", `"a b`, []string{`"a b`}},
		{"collapses repeated spaces", "a   b", []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizeFTS(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("tokenizeFTS(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNeedsQuoting(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"plain word", "hello", false},
		{"hyphenated", "aigc-cli", true},
		{"dotted", "file.md", true},
		{"asterisk suffix", "cat*", false},
		{"empty", "", false},
		{"plus sign", "a+b", false},
		{"quoted term", `"a b"`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsQuoting(tt.input); got != tt.want {
				t.Errorf("needsQuoting(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStripFTSReserved(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		keepHyphen bool
		want       string
	}{
		{"strips hyphen", "aigc-cli", false, "aigc cli"},
		{"keeps hyphen when asked", "aigc-cli", true, "aigc-cli"},
		{"strips quotes", `"hello"`, false, "hello"},
		{"strips operators", "a*b(c)~d", false, "a b c d"},
		{"strips plus", "c++", false, "c"},
		{"keeps dots", "file.md", false, "file.md"},
		{"collapses whitespace", "a   b", false, "a b"},
		{"empty", "", false, ""},
		{"all reserved", "*+~()", false, ""},
		{"keep hyphen does not keep other operators", "a-b*c", true, "a-b c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripFTSReserved(tt.input, tt.keepHyphen); got != tt.want {
				t.Errorf("stripFTSReserved(%q, %v) = %q, want %q", tt.input, tt.keepHyphen, got, tt.want)
			}
		})
	}
}

func TestSimplifyFTSQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"two long terms unchanged", "hello world", "hello world"},
		{"drops one and two char terms", "hello ab c world", "hello world"},
		{"all terms too short", "ab cd", ""},
		{"empty", "", ""},
		{"single long term", "hello", "hello"},
		{"keeps asterisk suffix", "cat* dog*", "cat* dog*"},
		{"three char terms kept", "abc def", "abc def"},
		{"short asterisk terms dropped", "ab* cd*", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simplifyFTSQuery(tt.input); got != tt.want {
				t.Errorf("simplifyFTSQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOrFTSQuery(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"two terms", "hello world", "hello OR world"},
		{"three terms", "alpha beta gamma", "alpha OR beta OR gamma"},
		{"single term returns empty", "hello", ""},
		{"empty returns empty", "", ""},
		{"drops short terms", "hello ab world", "hello OR world"},
		{"only one long term remains", "hello ab", ""},
		{"asterisk suffix kept", "cat* dog*", "cat* OR dog*"},
		{"all terms too short", "ab cd", ""},
		{"one long among short", "a bc def", ""},
		{"exactly two long terms among short", "one two ab", "one OR two"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := orFTSQuery(tt.input); got != tt.want {
				t.Errorf("orFTSQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSortResults(t *testing.T) {
	t.Run("sorts by score descending", func(t *testing.T) {
		results := SearchResults{
			{Chunk: Chunk{ID: 1}, Score: 0.1},
			{Chunk: Chunk{ID: 2}, Score: 0.9},
			{Chunk: Chunk{ID: 3}, Score: 0.5},
		}
		sortResults(results)
		wantScores := []float64{0.9, 0.5, 0.1}
		for i, want := range wantScores {
			if results[i].Score != want {
				t.Errorf("results[%d].Score = %v, want %v", i, results[i].Score, want)
			}
		}
		if results[0].Chunk.ID != 2 {
			t.Errorf("highest scored chunk ID = %d, want 2", results[0].Chunk.ID)
		}
	})

	t.Run("empty slice does not panic", func(t *testing.T) {
		sortResults(nil)
		sortResults(SearchResults{})
	})
}

func emb(vals ...float32) Embedding { return Embedding(vals) }

func TestEmbeddingBlobEdgeCases(t *testing.T) {
	t.Run("nil blob yields empty embedding", func(t *testing.T) {
		if got := blobToEmbedding(nil); len(got) != 0 {
			t.Errorf("blobToEmbedding(nil) = %v, want empty", got)
		}
	})

	t.Run("blob length matches embedding dimension", func(t *testing.T) {
		e := make(Embedding, 384)
		e[0] = 1.5
		e[383] = -2.25
		blob := embeddingToBlob(e)
		if len(blob) != 384*4 {
			t.Fatalf("embeddingToBlob length = %d, want %d", len(blob), 384*4)
		}
		if !reflect.DeepEqual(blobToEmbedding(blob), e) {
			t.Error("roundtrip through blob changed embedding values")
		}
	})

	t.Run("partial blob fills leading elements", func(t *testing.T) {
		blob := embeddingToBlob(emb(3, 4))
		got := blobToEmbedding(blob)
		if len(got) != 2 || got[0] != 3 || got[1] != 4 {
			t.Errorf("blobToEmbedding = %v, want (3, 4)", got)
		}
	})

	t.Run("blob decodes every float it holds", func(t *testing.T) {
		blob := append(embeddingToBlob(emb(1, 2)), embeddingToBlob(emb(3, 4))...)
		got := blobToEmbedding(blob)
		if len(got) != 4 || !reflect.DeepEqual(got, emb(1, 2, 3, 4)) {
			t.Errorf("blobToEmbedding = %v, want (1, 2, 3, 4)", got)
		}
	})
}

func TestCosineSimilarityEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		a    Embedding
		b    Embedding
		want float64
	}{
		{"zero vs zero", Embedding{}, Embedding{}, 0},
		{"zero vs unit", Embedding{}, emb(1), 0},
		{"identical", emb(1), emb(1), 1},
		{"opposite", emb(1), emb(-1), -1},
		{"orthogonal", emb(1, 0), emb(0, 1), 0},
		{"mismatched dimensions", emb(1, 2, 3), emb(1, 2), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if diff := got - tt.want; diff > 1e-6 || diff < -1e-6 {
				t.Errorf("cosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsFTS5DuplicateNonSQLiteError(t *testing.T) {
	if isFTS5Duplicate(nil) {
		t.Error("isFTS5Duplicate(nil) = true, want false")
	}
	if isFTS5Duplicate(fmt.Errorf("not a sqlite error")) {
		t.Error("isFTS5Duplicate(non-sqlite error) = true, want false")
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		name string
		v    int
		low  int
		high int
		want int
	}{
		{"below low", 0, 1, 10, 1},
		{"above high", 20, 1, 10, 10},
		{"within range", 5, 1, 10, 5},
		{"at low bound", 1, 1, 10, 1},
		{"at high bound", 10, 1, 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clamp(tt.v, tt.low, tt.high); got != tt.want {
				t.Errorf("clamp(%d, %d, %d) = %d, want %d", tt.v, tt.low, tt.high, got, tt.want)
			}
		})
	}
}
