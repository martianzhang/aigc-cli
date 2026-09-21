package vision

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// This file adds white-box tests for the pure helpers in the vision package:
// variant metadata, model file URL construction, readiness checks, image
// preprocessing, and the GPT-2 byte-level BPE tokenizer. No ONNX Runtime,
// network access or model files on disk are required.

// ── variant metadata / model files ──

func TestVariantInfos(t *testing.T) {
	info, ok := variantInfos[DefaultModelVariant]
	if !ok {
		t.Fatalf("variantInfos missing default variant %q", DefaultModelVariant)
	}
	if info.prefix != "vision_base-int8" {
		t.Errorf("prefix = %q, want %q", info.prefix, "vision_base-int8")
	}
	if info.desc == "" || info.size == "" {
		t.Errorf("variant info has empty desc/size: %+v", info)
	}
	if _, ok := variantInfos["does-not-exist"]; ok {
		t.Error("variantInfos unexpectedly contains unknown variant")
	}
}

func TestModelFilesURLs(t *testing.T) {
	files, err := ModelFiles(DefaultModelVariant)
	if err != nil {
		t.Fatalf("ModelFiles error: %v", err)
	}

	base := modelsBaseURL + "/vision_base-int8"
	want := map[string]string{
		"vision_encoder.onnx": base + "_vision_encoder.onnx",
		"encoder_model.onnx":  base + "_encoder_model.onnx",
		"decoder_model.onnx":  base + "_decoder_model.onnx",
		"embed_tokens.onnx":   base + "_embed_tokens.onnx",
		"vocab.json":          base + "_vocab.json",
		"merges.txt":          base + "_merges.txt",
	}

	if len(files) != len(want) {
		t.Fatalf("ModelFiles returned %d files, want %d", len(files), len(want))
	}
	for _, f := range files {
		url, ok := want[f.Filename]
		if !ok {
			t.Errorf("unexpected filename %q", f.Filename)
			continue
		}
		if f.URL != url {
			t.Errorf("URL for %s = %q, want %q", f.Filename, f.URL, url)
		}
		if f.Size == "" {
			t.Errorf("file %s has empty Size", f.Filename)
		}
	}
}

func TestIsReady(t *testing.T) {
	modelsDir := t.TempDir()
	variant := DefaultModelVariant
	dir := VariantDir(modelsDir, variant)

	if IsReady(modelsDir, variant) {
		t.Error("IsReady(empty dir) = true, want false")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if IsReady(modelsDir, variant) {
		t.Error("IsReady(dir without models) = true, want false")
	}

	for _, name := range []string{
		"vision_encoder.onnx", "embed_tokens.onnx",
		"encoder_model.onnx", "decoder_model.onnx",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if !IsReady(modelsDir, variant) {
		t.Error("IsReady(all files) = false, want true")
	}

	if err := os.Remove(filepath.Join(dir, "decoder_model.onnx")); err != nil {
		t.Fatal(err)
	}
	if IsReady(modelsDir, variant) {
		t.Error("IsReady(missing one file) = true, want false")
	}
}

// ── image preprocessing ──

func normalizedValue(rgba uint32, channel int) float64 {
	v := float64(rgba) / 65535.0
	return (v - float64(MeanRGB[channel])) / float64(StdRGB[channel])
}

func TestPreprocessImageFromImage(t *testing.T) {
	const size = 2
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, white)
		}
	}

	pixels, err := PreprocessImageFromImage(img, size)
	if err != nil {
		t.Fatalf("PreprocessImageFromImage error: %v", err)
	}
	if want := 3 * size * size; len(pixels) != want {
		t.Fatalf("pixel count = %d, want %d", len(pixels), want)
	}

	// White pixel: normalized (1-mean)/std for each channel in CHW order.
	for c := 0; c < 3; c++ {
		want := float32(normalizedValue(65535, c))
		if got := pixels[c*size*size]; math.Abs(float64(got-want)) > 1e-4 {
			t.Errorf("channel %d white pixel = %v, want %v", c, got, want)
		}
	}

	// A black pixel normalizes to (0-mean)/std (negative for all channels).
	black := image.NewRGBA(image.Rect(0, 0, 1, 1))
	black.Set(0, 0, color.RGBA{A: 255})
	bp, err := PreprocessImageFromImage(black, 1)
	if err != nil {
		t.Fatalf("black preprocess error: %v", err)
	}
	for c := 0; c < 3; c++ {
		want := float32(normalizedValue(0, c))
		if math.Abs(float64(bp[c]-want)) > 1e-4 {
			t.Errorf("channel %d black pixel = %v, want %v", c, bp[c], want)
		}
	}
}

func TestPreprocessImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "img.png")

	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 60), B: 128, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, src); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	pixels, err := PreprocessImage(path)
	if err != nil {
		t.Fatalf("PreprocessImage error: %v", err)
	}
	if want := 3 * InputSize * InputSize; len(pixels) != want {
		t.Errorf("pixel count = %d, want %d", len(pixels), want)
	}

	if _, err := PreprocessImage(filepath.Join(dir, "missing.png")); err == nil {
		t.Error("PreprocessImage(missing) = nil error, want error")
	}

	bad := filepath.Join(dir, "bad.png")
	if err := os.WriteFile(bad, []byte("not an image"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := PreprocessImage(bad); err == nil {
		t.Error("PreprocessImage(invalid) = nil error, want error")
	}
}

// ── GPT-2 BPE tokenizer ──

func writeBPEFiles(t *testing.T, vocab map[string]int, merges []string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	vocabPath := filepath.Join(dir, "vocab.json")
	mergesPath := filepath.Join(dir, "merges.txt")

	data, err := json.Marshal(vocab)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vocabPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mergesPath, []byte(strings.Join(merges, "\n")), 0644); err != nil {
		t.Fatal(err)
	}
	return vocabPath, mergesPath
}

func TestBytesToUnicode(t *testing.T) {
	m := bytesToUnicode()
	if len(m) != 256 {
		t.Fatalf("bytesToUnicode size = %d, want 256", len(m))
	}
	if m['A'] != "A" {
		t.Errorf("byte 'A' = %q, want %q", m['A'], "A")
	}
	if m[0] != string(rune(256)) {
		t.Errorf("byte 0 = %q, want %q", m[0], string(rune(256)))
	}
	seen := make(map[string]bool, 256)
	for b, s := range m {
		if seen[s] {
			t.Errorf("duplicate mapping for byte %d: %q", b, s)
		}
		seen[s] = true
	}
}

func TestBuildByteEncoderInverse(t *testing.T) {
	tk := &Tokenizer{}
	tk.buildByteEncoder()
	if len(tk.byteEncoder) != 256 || len(tk.byteDecoder) != 256 {
		t.Fatalf("encoder/decoder sizes = %d/%d, want 256/256",
			len(tk.byteEncoder), len(tk.byteDecoder))
	}
	for b := 0; b < 256; b++ {
		ch := tk.byteEncoder[byte(b)]
		if got := tk.byteDecoder[ch]; got != byte(b) {
			t.Errorf("roundtrip byte %d → %q → %d", b, ch, got)
		}
	}
}

func TestBPETokenizerEncodeDecode(t *testing.T) {
	vocab := map[string]int{"A": 10, "B": 11, "C": 12, "AB": 13}
	vocabPath, mergesPath := writeBPEFiles(t, vocab, []string{"A B"})

	tk, err := NewTokenizer(vocabPath, mergesPath)
	if err != nil {
		t.Fatalf("NewTokenizer error: %v", err)
	}
	if tk.VocabSize() != 4 {
		t.Errorf("VocabSize = %d, want 4", tk.VocabSize())
	}

	if got := tk.Encode("A"); !equalInt64(got, []int64{0, 10, 2}) {
		t.Errorf("Encode(A) = %v, want [0 10 2]", got)
	}
	// "ABC" is not a vocab token: BPE merges "AB", leaving "C".
	if got := tk.Encode("ABC"); !equalInt64(got, []int64{0, 13, 12, 2}) {
		t.Errorf("Encode(ABC) = %v, want [0 13 12 2]", got)
	}
	if got := tk.Encode(""); !equalInt64(got, []int64{0, 2}) {
		t.Errorf("Encode(\"\") = %v, want [0 2]", got)
	}

	if got := tk.EncodeTokens("ABC"); !equalStrings(got, []string{"AB", "C"}) {
		t.Errorf("EncodeTokens(ABC) = %v, want [AB C]", got)
	}

	if got := tk.Decode([]int64{0, 13, 12, 2}); got != "ABC" {
		t.Errorf("Decode = %q, want %q", got, "ABC")
	}
	if got := tk.Decode([]int64{0, 10, 2}); got != "A" {
		t.Errorf("Decode(A) = %q, want %q", got, "A")
	}
	if got := tk.Decode(nil); got != "" {
		t.Errorf("Decode(nil) = %q, want empty", got)
	}
}

func TestBPE(t *testing.T) {
	vocabPath, mergesPath := writeBPEFiles(t, map[string]int{"A": 1}, []string{"A B"})
	tk, err := NewTokenizer(vocabPath, mergesPath)
	if err != nil {
		t.Fatalf("NewTokenizer error: %v", err)
	}

	if got := tk.bpe("A"); !equalStrings(got, []string{"A"}) {
		t.Errorf("bpe(single char) = %v, want [A]", got)
	}
	if got := tk.bpe("XYZ"); !equalStrings(got, []string{"X", "Y", "Z"}) {
		t.Errorf("bpe(no merges) = %v, want [X Y Z]", got)
	}
	// Cached result must be stable across calls.
	first := tk.bpe("XYZ")
	second := tk.bpe("XYZ")
	if !equalStrings(first, second) {
		t.Errorf("bpe cache inconsistent: %v vs %v", first, second)
	}
}

func TestLoadMergesSkipsCommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	vocabPath := filepath.Join(dir, "vocab.json")
	mergesPath := filepath.Join(dir, "merges.txt")

	if err := os.WriteFile(vocabPath, []byte(`{"A":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	content := "#version: 0.2\nA B\n\nX Y\n"
	if err := os.WriteFile(mergesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tk, err := NewTokenizer(vocabPath, mergesPath)
	if err != nil {
		t.Fatalf("NewTokenizer error: %v", err)
	}
	if rank, ok := tk.bpeRanks["A B"]; !ok || rank != 1 {
		t.Errorf("rank(A B) = %d ok=%v, want 1", rank, ok)
	}
	if rank, ok := tk.bpeRanks["X Y"]; !ok || rank != 3 {
		t.Errorf("rank(X Y) = %d ok=%v, want 3", rank, ok)
	}
	if _, ok := tk.bpeRanks["#version: 0.2"]; ok {
		t.Error("comment line should not be registered as a merge")
	}
}

func TestBPETokenizerInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	vocabPath := filepath.Join(dir, "vocab.json")
	mergesPath := filepath.Join(dir, "merges.txt")

	if _, err := NewTokenizer(filepath.Join(dir, "missing.json"), mergesPath); err == nil {
		t.Error("NewTokenizer(missing vocab) = nil error, want error")
	}

	if err := os.WriteFile(vocabPath, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mergesPath, []byte("\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTokenizer(vocabPath, mergesPath); err == nil {
		t.Error("NewTokenizer(invalid vocab) = nil error, want error")
	}

	if err := os.WriteFile(vocabPath, []byte(`{"A":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewTokenizer(vocabPath, filepath.Join(dir, "missing-merges.txt")); err == nil {
		t.Error("NewTokenizer(missing merges) = nil error, want error")
	}
}

// ── ListModels ──

func TestListModels(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	ListModels()
	_ = w.Close()
	os.Stdout = old

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "base-int8") {
		t.Errorf("ListModels output missing variant ID: %q", text)
	}
	if !strings.Contains(text, "(default)") {
		t.Errorf("ListModels output missing default marker: %q", text)
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
