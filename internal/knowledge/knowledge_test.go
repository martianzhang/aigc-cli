package knowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestHashEmbedder_Deterministic(t *testing.T) {
	e := NewHashEmbedder(384)
	v1, err := e.Embed("hello world")
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	v2, err := e.Embed("hello world")
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if v1 != v2 {
		t.Error("embeddings not deterministic for same input")
	}
}

func TestHashEmbedder_Dimension(t *testing.T) {
	e := NewHashEmbedder(384)
	if d := e.Dim(); d != 384 {
		t.Errorf("expected dim 384, got %d", d)
	}
}

func TestHashEmbedder_DifferentInputs(t *testing.T) {
	e := NewHashEmbedder(384)
	v1, _ := e.Embed("cat")
	v2, _ := e.Embed("dog")
	if v1 == v2 {
		t.Error("different inputs should produce different embeddings")
	}
}

func TestHashEmbedder_Empty(t *testing.T) {
	e := NewHashEmbedder(384)
	_, err := e.Embed("")
	if err != nil {
		t.Fatalf("embed empty: %v", err)
	}
}

func TestHashEmbedder_Normalized(t *testing.T) {
	e := NewHashEmbedder(384)
	v, _ := e.Embed("test document for normalization check")
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("expected unit vector (norm≈1), got norm=%f", norm)
	}
}

func TestChunker_Basic(t *testing.T) {
	c := NewChunker(DefaultChunkOptions())
	text := "## Introduction\n\nThis is the introduction.\n\n## Methods\n\nThis describes the methods."
	chunks := c.Chunk(text)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if chunks[0].Heading == "" {
		t.Error("expected heading in first chunk")
	}
}

func TestChunker_SmallContent(t *testing.T) {
	c := NewChunker(DefaultChunkOptions())
	chunks := c.Chunk("Small piece of text without headings.")
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

func TestChunker_Empty(t *testing.T) {
	c := NewChunker(DefaultChunkOptions())
	chunks := c.Chunk("")
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks, got %d", len(chunks))
	}
}

func TestChecksum(t *testing.T) {
	c1 := Checksum("hello")
	c2 := Checksum("hello")
	c3 := Checksum("world")
	if c1 != c2 {
		t.Error("same content should produce same checksum")
	}
	if c1 == c3 {
		t.Error("different content should produce different checksum")
	}
}

func TestDocID(t *testing.T) {
	id := DocID("test content")
	if id == "" {
		t.Error("doc ID should not be empty")
	}
	if len(id) != 64 {
		t.Errorf("expected 64-char SHA256, got %d", len(id))
	}
}

func TestParseSearchQuery(t *testing.T) {
	tests := []struct {
		input   string
		wantFTS bool
	}{
		{"hello world", true},
		{"test query with special chars !@#$%", true},
	}
	for _, tt := range tests {
		sq := ParseSearchQuery(tt.input)
		if tt.wantFTS && sq.Processed == "" {
			t.Errorf("ParseSearchQuery(%q) produced empty processed query", tt.input)
		}
	}
	// Empty query should produce empty result
	sq := ParseSearchQuery("")
	if sq.Processed != "" {
		t.Errorf("empty query should produce empty processed, got %q", sq.Processed)
	}
}

func TestCosineSimilarity(t *testing.T) {
	var a, b Embedding
	a[0] = 1
	b[1] = 1
	s := cosineSimilarity(a, b)
	if s > 0.01 {
		t.Errorf("orthogonal vectors should have near-zero similarity, got %f", s)
	}
	s = cosineSimilarity(a, a)
	if s < 0.99 {
		t.Errorf("same vector should have near-1 similarity, got %f", s)
	}
}

func TestStore_OpenAndClose(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir, 384, nil)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	c, err := s.CountDocuments()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if c != 0 {
		t.Errorf("expected 0 docs, got %d", c)
	}
}

func TestStore_SaveAndGetDocument(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir, 384, nil)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	doc := &Document{
		ID: "abc123", URL: "https://example.com",
		Title: "Test Doc", Size: 100, Checksum: "chk123",
	}
	if err := s.SaveDocument(doc); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := s.GetDocument("abc123")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Test Doc" {
		t.Errorf("expected 'Test Doc', got %q", got.Title)
	}
}

func TestStore_SaveChunks(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir, 384, nil)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	doc := &Document{ID: "doc1", Title: "Test", Size: 50, Checksum: "chk"}
	if err := s.SaveDocument(doc); err != nil {
		t.Fatalf("save doc: %v", err)
	}
	embedder := NewHashEmbedder(384)
	chunks := []Chunk{
		{Index: 0, Content: "First chunk content", Heading: "## First"},
		{Index: 1, Content: "Second chunk content", Heading: "## Second"},
	}
	embeddings := make([]Embedding, len(chunks))
	for i, c := range chunks {
		embeddings[i], err = embedder.Embed(c.Content)
		if err != nil {
			t.Fatalf("embed: %v", err)
		}
	}
	if err := s.SaveChunks("doc1", chunks, embeddings, false); err != nil {
		t.Fatalf("save chunks: %v", err)
	}
}

func TestStore_DeleteDocument(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir, 384, nil)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	s.SaveDocument(&Document{ID: "del_doc", Title: "To Delete", Size: 10, Checksum: "x"})
	if err := s.DeleteDocument("del_doc"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := s.GetDocument("del_doc")
	if got != nil {
		t.Error("document should be nil after delete")
	}
}

func TestStore_ListDocuments(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenStore(dir, 384, nil)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()
	docs, err := s.ListDocuments(10, 0, "")
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0, got %d", len(docs))
	}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("doc_%d", i)
		s.SaveDocument(&Document{ID: id, Title: "Doc " + id, Size: 10, Checksum: id})
	}
	docs, err = s.ListDocuments(10, 0, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(docs) != 3 {
		t.Errorf("expected 3, got %d", len(docs))
	}
}

func TestEmbeddingBlobRoundtrip(t *testing.T) {
	var orig Embedding
	for i := 0; i < 384; i++ {
		orig[i] = float32(i) / 384.0
	}
	blob := embeddingToBlob(orig)
	restored := blobToEmbedding(blob)
	if orig != restored {
		t.Error("embedding roundtrip produced different result")
	}
}

func TestWordCount(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
	}
	for _, tt := range tests {
		if got := WordCount(tt.input); got != tt.want {
			t.Errorf("WordCount(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestCountTokens(t *testing.T) {
	if n := CountTokens("hello"); n == 0 {
		t.Error("expected non-zero token count")
	}
}

func TestNormalizeEmbedding(t *testing.T) {
	var e Embedding
	e[0] = 3
	e[1] = 4
	NormalizeEmbedding(&e)
	var norm float64
	for _, v := range e {
		norm += float64(v) * float64(v)
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("expected norm≈1, got %f", norm)
	}
}

func TestMeanEmbedding(t *testing.T) {
	var e1, e2 Embedding
	e1[1] = 1
	e1[2] = 2
	e2[1] = 3
	e2[4] = 4
	mean := MeanEmbedding([]Embedding{e1, e2})
	if mean[1] != 2.0 || mean[2] != 1.0 || mean[4] != 2.0 {
		t.Errorf("unexpected mean values: %v", mean[:5])
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.md"), []byte("# Test"), 0644)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("text"), 0644)
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "test.bin"), []byte{0, 1, 2}, 0644)
	files, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 supported files, got %d", len(files))
	}
}

func TestDetectFileType(t *testing.T) {
	tests := []struct {
		path string
		want FileType
	}{
		{"file.md", FileTypeMarkdown},
		{"file.txt", FileTypeText},
		{"file.go", FileTypeCode},
		{"file.json", FileTypeJSON},
		{"file.yaml", FileTypeYAML},
		{"file.html", FileTypeHTML},
		{"file.pdf", FileTypePDF},
		{"file.docx", FileTypeDocx},
		{"file.unknown", FileTypeUnknown},
	}
	for _, tt := range tests {
		if got := DetectFileType(tt.path); got != tt.want {
			t.Errorf("DetectFileType(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsHeading(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"# Title", true},
		{"## Section", true},
		{"### Subsection", true},
		{"Not a heading", false},
		{"", false},
		{"#NoSpace", false},
	}
	for _, tt := range tests {
		if got := isHeading(tt.line); got != tt.want {
			t.Errorf("isHeading(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestTokenizeWords(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello world", 2},
		{"hello-world test", 2},
		{"don't split", 2},
		{"", 0},
	}
	for _, tt := range tests {
		words := tokenizeWords(tt.input)
		if len(words) != tt.want {
			t.Errorf("tokenizeWords(%q) got %d words, want %d: %v", tt.input, len(words), tt.want, words)
		}
	}
}

func TestRRFFuse(t *testing.T) {
	a := SearchResults{
		{Chunk: Chunk{ID: 1, Content: "first"}, Score: 0.9},
		{Chunk: Chunk{ID: 2, Content: "second"}, Score: 0.8},
	}
	b := SearchResults{
		{Chunk: Chunk{ID: 2, Content: "second"}, Score: 0.85},
		{Chunk: Chunk{ID: 3, Content: "third"}, Score: 0.7},
	}
	merged := rrfFuse(a, b, 3)
	if len(merged) != 3 {
		t.Errorf("expected 3 merged results, got %d", len(merged))
	}
	// First should be "second" (ranked high in both lists)
	if merged[0].Chunk.Content != "second" {
		t.Errorf("expected 'second' first by RRF, got %q", merged[0].Chunk.Content)
	}
}
