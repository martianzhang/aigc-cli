package knowledge

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// This file adds white-box tests for the remaining pure helpers in the
// knowledge package: config/struct round-trips, model file URL construction,
// path/slug construction, plain-text detection, the local file loader, the
// unigram tokenizer, and the fetch guard client helpers. No SQLite, network or
// ONNX runtime is involved.

// ── config / model structs ──

func TestDefaultChunkOptionsValues(t *testing.T) {
	opts := DefaultChunkOptions()
	if opts.MaxSize != 2048 {
		t.Errorf("MaxSize = %d, want 2048", opts.MaxSize)
	}
	if opts.Overlap != 128 {
		t.Errorf("Overlap = %d, want 128", opts.Overlap)
	}
	if !opts.SplitCode {
		t.Error("SplitCode = false, want true")
	}
}

func TestChunkOptionsZeroValue(t *testing.T) {
	var opts ChunkOptions
	if opts.MaxSize != 0 || opts.Overlap != 0 || opts.SplitCode {
		t.Errorf("zero ChunkOptions = %+v, want all zero", opts)
	}
}

func TestKBConfigZeroValue(t *testing.T) {
	var cfg KBConfig
	if cfg.BaseDir != "" {
		t.Errorf("zero KBConfig.BaseDir = %q, want empty", cfg.BaseDir)
	}
	cfg.BaseDir = "/tmp/kb"
	if cfg.BaseDir != "/tmp/kb" {
		t.Errorf("KBConfig.BaseDir roundtrip = %q", cfg.BaseDir)
	}
}

func TestSearchQueryStructRoundtrip(t *testing.T) {
	var zero SearchQuery
	if zero.Raw != "" || zero.Processed != "" {
		t.Errorf("zero SearchQuery = %+v, want empty", zero)
	}
	sq := SearchQuery{Raw: "raw text", Processed: "processed text"}
	if sq.Raw != "raw text" || sq.Processed != "processed text" {
		t.Errorf("SearchQuery roundtrip = %+v", sq)
	}
}

func TestCoreStructRoundtrips(t *testing.T) {
	now := time.Now()
	doc := Document{
		ID: "id", URL: "https://example.com", FilePath: "a.md",
		Title: "t", Project: "p", IsVault: true, Size: 7,
		Checksum: "ck", CreatedAt: now, UpdatedAt: now,
	}
	if doc.ID != "id" || doc.URL != "https://example.com" || doc.FilePath != "a.md" ||
		doc.Title != "t" || doc.Project != "p" || !doc.IsVault || doc.Size != 7 ||
		doc.Checksum != "ck" || !doc.CreatedAt.Equal(now) || !doc.UpdatedAt.Equal(now) {
		t.Errorf("Document roundtrip mismatch: %+v", doc)
	}

	chunk := Chunk{ID: 1, DocID: "id", Index: 2, Content: "c", Heading: "h", CreatedAt: now}
	if chunk.ID != 1 || chunk.DocID != "id" || chunk.Index != 2 ||
		chunk.Content != "c" || chunk.Heading != "h" || !chunk.CreatedAt.Equal(now) {
		t.Errorf("Chunk roundtrip mismatch: %+v", chunk)
	}

	var emb Embedding
	emb[0] = 1.5
	emb[383] = -2.5
	if emb[0] != 1.5 || emb[383] != -2.5 {
		t.Errorf("Embedding roundtrip mismatch")
	}

	var sr SearchResult
	if sr.Score != 0 {
		t.Errorf("zero SearchResult.Score = %v, want 0", sr.Score)
	}
}

// ── embedding model files / paths ──

func TestEmbedModelFiles(t *testing.T) {
	files := EmbedModelFiles()
	if len(files) != 2 {
		t.Fatalf("EmbedModelFiles() returned %d files, want 2", len(files))
	}

	want := map[string]int64{
		"model.onnx":     110,
		"tokenizer.json": 16,
	}
	for _, f := range files {
		size, ok := want[f.OutName]
		if !ok {
			t.Errorf("unexpected model file %q", f.OutName)
			continue
		}
		if !strings.HasPrefix(f.URL, modelsBaseURL+"/") {
			t.Errorf("URL %q does not start with models base URL", f.URL)
		}
		if !strings.HasSuffix(f.URL, f.OutName) {
			t.Errorf("URL %q does not end with OutName %q", f.URL, f.OutName)
		}
		if f.SizeMB != size {
			t.Errorf("SizeMB for %s = %d, want %d", f.OutName, f.SizeMB, size)
		}
	}
}

func TestModelFileZeroValue(t *testing.T) {
	var f ModelFile
	if f.URL != "" || f.OutName != "" || f.SizeMB != 0 {
		t.Errorf("zero ModelFile = %+v, want empty", f)
	}
}

func TestEmbedModelDir(t *testing.T) {
	got := EmbedModelDir("/models")
	want := filepath.Join("/models", EmbedModelID)
	if got != want {
		t.Errorf("EmbedModelDir = %q, want %q", got, want)
	}
	if got := EmbedModelDir(""); got != EmbedModelID {
		t.Errorf("EmbedModelDir(\"\") = %q, want %q", got, EmbedModelID)
	}
}

// ── path / slug helpers ──

func TestProjectDir(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"simple", "simple"},
		{"With-Caps_1.2", "With-Caps_1.2"},
		{"spaces and/slashes", "spaces_and_slashes"},
		{"中文项目", "____"},
		{"a!b@c", "a_b_c"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := projectDir(tt.in); got != tt.want {
			t.Errorf("projectDir(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Hello World", "hello-world"},
		{"Hello, World!", "hello-world"},
		{"a  b", "a-b"},
		{"UP_Case", "up-case"},
		{"already-slugged", "already-slugged"},
		{"中文标题", ""},
		{"", ""},
		{"ABC123", "abc123"},
	}
	for _, tt := range tests {
		if got := slugify(tt.in); got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsText(t *testing.T) {
	if !isText([]byte("plain text")) {
		t.Error("isText(plain) = false, want true")
	}
	if !isText([]byte("utf-8 中文 ✅")) {
		t.Error("isText(utf-8) = false, want true")
	}
	if isText([]byte{0x00}) {
		t.Error("isText(null byte) = true, want false")
	}
	if isText([]byte("abc\x00def")) {
		t.Error("isText(embedded null) = true, want false")
	}
	if !isText(nil) {
		t.Error("isText(nil) = false, want true")
	}
}

// ── LoadFile ──

func TestLoadFileFormats(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return p
	}

	tests := []struct {
		file        string
		content     string
		wantTitle   string
		wantContent string
	}{
		{"a.md", "# Title\nbody", "a.md", "# Title\nbody"},
		{"a.txt", "plain", "a.txt", "plain"},
		{"a.go", "package main", "a.go", "```go\npackage main\n```"},
		{"a.json", `{"k":1}`, "a.json", "```json\n{\"k\":1}\n```"},
		{"a.yaml", "k: v", "a.yaml", "```yaml\nk: v\n```"},
	}
	for _, tt := range tests {
		p := write(tt.file, tt.content)
		title, content, err := LoadFile(p)
		if err != nil {
			t.Errorf("LoadFile(%s) error: %v", tt.file, err)
			continue
		}
		if title != tt.wantTitle {
			t.Errorf("LoadFile(%s) title = %q, want %q", tt.file, title, tt.wantTitle)
		}
		if content != tt.wantContent {
			t.Errorf("LoadFile(%s) content = %q, want %q", tt.file, content, tt.wantContent)
		}
	}

	htmlPath := write("a.html", "<h1>Hi</h1>")
	_, content, err := LoadFile(htmlPath)
	if err != nil {
		t.Fatalf("LoadFile(html) error: %v", err)
	}
	if !strings.Contains(content, "```html") || !strings.Contains(content, "Source: ") {
		t.Errorf("LoadFile(html) content missing html wrapper: %q", content)
	}

	unknownText := write("a.xyz", "just text")
	if _, content, err := LoadFile(unknownText); err != nil || content != "just text" {
		t.Errorf("LoadFile(unknown text) = (%q, %v), want plain text", content, err)
	}

	unknownBinary := write("b.xyz", "bin\x00")
	if _, _, err := LoadFile(unknownBinary); err == nil {
		t.Error("LoadFile(binary) = nil error, want unsupported type error")
	}
}

func TestLoadFileUnsupportedAndMissing(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "doc.pdf")
	if err := os.WriteFile(pdf, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadFile(pdf); err == nil {
		t.Error("LoadFile(pdf) = nil error, want error")
	}

	docx := filepath.Join(dir, "doc.docx")
	if err := os.WriteFile(docx, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadFile(docx); err == nil {
		t.Error("LoadFile(docx) = nil error, want error")
	}

	if _, _, err := LoadFile(filepath.Join(dir, "does-not-exist.md")); err == nil {
		t.Error("LoadFile(missing) = nil error, want error")
	}
}

func TestRunExternalLoader(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cat is not available on Windows")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(p, []byte("loaded body"), 0644); err != nil {
		t.Fatal(err)
	}

	title, content, err := RunExternalLoader("cat $1", p)
	if err != nil {
		t.Fatalf("RunExternalLoader error: %v", err)
	}
	if title != "doc.md" {
		t.Errorf("title = %q, want doc.md", title)
	}
	if content != "loaded body" {
		t.Errorf("content = %q, want %q", content, "loaded body")
	}

	if _, _, err := RunExternalLoader("   ", p); err == nil {
		t.Error("RunExternalLoader(empty cmd) = nil error, want error")
	}
	if _, _, err := RunExternalLoader("no-such-loader-bin-xyz $1", p); err == nil {
		t.Error("RunExternalLoader(missing binary) = nil error, want error")
	}
}

// ── unigram tokenizer (embed_tokenizer.go) ──

func newTestTokenizer(t *testing.T, payload map[string]any) *Tokenizer {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal tokenizer fixture: %v", err)
	}
	tk, err := NewTokenizer(data)
	if err != nil {
		t.Fatalf("NewTokenizer: %v", err)
	}
	return tk
}

func unigramPayload(vocab any, scores []float64) map[string]any {
	return map[string]any{
		"version": "test",
		"model": map[string]any{
			"type":   "Unigram",
			"vocab":  vocab,
			"scores": scores,
		},
	}
}

func TestTokenizerObjectVocabAndEncode(t *testing.T) {
	vocab := map[string]int{
		"<unk>": 0, "<s>": 1, "</s>": 2, "<pad>": 3,
		"a": 4, "b": 5, "ab": 6, "c": 7,
	}
	scores := []float64{0, 0, 0, 0, 0.5, 0.5, 1.0, 0.4}
	tk := newTestTokenizer(t, unigramPayload(vocab, scores))

	if tk.VocabSize() != 8 {
		t.Errorf("VocabSize = %d, want 8", tk.VocabSize())
	}
	if tk.unkID != 0 || tk.bosID != 1 || tk.eosID != 2 || tk.padID != 3 {
		t.Errorf("special ids = (unk %d, bos %d, eos %d, pad %d), want (0,1,2,3)",
			tk.unkID, tk.bosID, tk.eosID, tk.padID)
	}

	if got := tk.Encode("ab"); !equalInt64(got, []int64{1, 6, 2}) {
		t.Errorf("Encode(ab) = %v, want [1 6 2]", got)
	}
	// "abc" is not a vocab token; Viterbi should split it into "ab" + "c".
	if got := tk.Encode("abc"); !equalInt64(got, []int64{1, 6, 7, 2}) {
		t.Errorf("Encode(abc) = %v, want [1 6 7 2]", got)
	}
	// Unknown word falls back to <unk>.
	if got := tk.Encode("zzz"); !equalInt64(got, []int64{1, 0, 2}) {
		t.Errorf("Encode(zzz) = %v, want [1 0 2]", got)
	}
}

func TestTokenizerDecode(t *testing.T) {
	vocab := map[string]int{
		"<unk>": 0, "<s>": 1, "</s>": 2, "<pad>": 3,
		"ab": 4, "c": 5, "▁hello": 6,
	}
	tk := newTestTokenizer(t, unigramPayload(vocab, nil))

	if got := tk.Decode([]int64{1, 4, 5, 2}); got != "abc" {
		t.Errorf("Decode = %q, want %q", got, "abc")
	}
	if got := tk.Decode([]int64{1, 2, 3}); got != "" {
		t.Errorf("Decode(special only) = %q, want empty", got)
	}
	if got := tk.Decode([]int64{6}); got != "hello" {
		t.Errorf("Decode(▁hello) = %q, want %q", got, "hello")
	}
	if got := tk.Decode([]int64{999}); got != "" {
		t.Errorf("Decode(unknown id) = %q, want empty", got)
	}
}

func TestPreTokenizeUnigram(t *testing.T) {
	got := preTokenizeUnigram("hello, world!")
	tests := []struct {
		want []string
	}{
		{[]string{"hello", ",", "world", "!"}},
	}
	if !equalStrings(got, tests[0].want) {
		t.Errorf("preTokenizeUnigram = %v, want %v", got, tests[0].want)
	}
	if got := preTokenizeUnigram(""); len(got) != 0 {
		t.Errorf("preTokenizeUnigram(\"\") = %v, want empty", got)
	}
}

func TestUnigramTokenizeEdgeCases(t *testing.T) {
	vocab := map[string]int{"a": 0, "b": 1, "ab": 2}
	tk := newTestTokenizer(t, unigramPayload(vocab, []float64{0.5, 0.5, 1.0}))

	if got := tk.unigramTokenize(""); got != nil {
		t.Errorf("unigramTokenize(\"\") = %v, want nil", got)
	}
	if got := tk.unigramTokenize("ab"); !equalStrings(got, []string{"ab"}) {
		t.Errorf("unigramTokenize(vocab word) = %v, want [ab]", got)
	}
	// "ab" + "c" is not in vocab; no valid segmentation exists in this vocab.
	if got := tk.unigramTokenize("abc"); !equalStrings(got, []string{"abc"}) {
		t.Errorf("unigramTokenize(no segmentation) = %v, want [abc]", got)
	}
}

func TestTokenizerArrayVocab(t *testing.T) {
	// Array format: [[token, score], ...]; ids are positional.
	vocab := []any{
		[]any{"a", 0.5},
		[]any{"b", 0.5},
		[]any{"ab", 1.0},
	}
	payload := map[string]any{
		"model": map[string]any{
			"type":  "Unigram",
			"vocab": vocab,
		},
	}
	tk := newTestTokenizer(t, payload)

	if tk.VocabSize() != 3 {
		t.Errorf("array vocab size = %d, want 3", tk.VocabSize())
	}
	if got := tk.unigramTokenize("ab"); !equalStrings(got, []string{"ab"}) {
		t.Errorf("array vocab unigramTokenize(ab) = %v, want [ab]", got)
	}
	if tk.scores["ab"] != 1.0 {
		t.Errorf("array vocab score for ab = %v, want 1.0", tk.scores["ab"])
	}
}

func TestTokenizerAddedTokens(t *testing.T) {
	payload := unigramPayload(map[string]int{"a": 0}, nil)
	payload["added_tokens"] = []map[string]any{
		{"id": 9, "content": "<extra>", "single_word": true},
	}
	tk := newTestTokenizer(t, payload)
	if id, ok := tk.vocab["<extra>"]; !ok || id != 9 {
		t.Errorf("added token not registered: id=%d ok=%v", id, ok)
	}
}

func TestTokenizerInvalidInput(t *testing.T) {
	if _, err := NewTokenizer([]byte("not json")); err == nil {
		t.Error("NewTokenizer(invalid json) = nil error, want error")
	}
	payload := map[string]any{
		"model": map[string]any{"vocab": 42},
	}
	data, _ := json.Marshal(payload)
	if _, err := NewTokenizer(data); err == nil {
		t.Error("NewTokenizer(non-object/array vocab) = nil error, want error")
	}
}

// ── fetch guard helpers ──

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.1.1", "0.0.0.0", "224.0.0.1", "::1", "fe80::1", "fc00::1",
	}
	for _, s := range blocked {
		if ip := net.ParseIP(s); ip != nil && !isBlockedIP(ip) {
			t.Errorf("isBlockedIP(%s) = false, want true", s)
		}
	}
	public := []string{"8.8.8.8", "93.184.216.34", "1.1.1.1", "2606:4700::1111"}
	for _, s := range public {
		if ip := net.ParseIP(s); ip != nil && isBlockedIP(ip) {
			t.Errorf("isBlockedIP(%s) = true, want false", s)
		}
	}
}

func TestNewKBClient(t *testing.T) {
	c := newKBClient(3 * time.Second)
	if c == http.DefaultClient {
		t.Error("newKBClient should return a distinct client")
	}
	if c.Timeout != 3*time.Second {
		t.Errorf("Timeout = %v, want 3s", c.Timeout)
	}
	if c.CheckRedirect == nil {
		t.Error("CheckRedirect = nil, want non-nil")
	}
}

func TestCheckRedirect(t *testing.T) {
	publicReq, err := http.NewRequest("GET", "http://93.184.216.34/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRedirect(publicReq, nil); err != nil {
		t.Errorf("checkRedirect(public) = %v, want nil", err)
	}

	blockedReq, err := http.NewRequest("GET", "http://127.0.0.1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkRedirect(blockedReq, nil); err == nil {
		t.Error("checkRedirect(loopback) = nil, want error")
	}

	via := make([]*http.Request, 10)
	if err := checkRedirect(publicReq, via); err == nil {
		t.Error("checkRedirect(10 redirects) = nil, want error")
	}
}

// ── small helpers ──

func equalInt64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
