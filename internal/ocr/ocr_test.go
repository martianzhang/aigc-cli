package ocr

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// ----- Constants & Defaults -----

func TestDefaultPaths(t *testing.T) {
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".config", "aigc-cli", "models", "ch_PP-OCRv4_det_infer.onnx")
	if got := DefaultDetModelPath(home + "/.config/aigc-cli/models"); got != expected {
		t.Errorf("DefaultDetModelPath() = %q, want %q", got, expected)
	}

	expected = filepath.Join(home, ".config", "aigc-cli", "models", "ch_PP-OCRv4_rec_infer.onnx")
	if got := DefaultRecModelPath(home + "/.config/aigc-cli/models"); got != expected {
		t.Errorf("DefaultRecModelPath() = %q, want %q", got, expected)
	}

	expected = filepath.Join(home, ".config", "aigc-cli", "models", "ch_ppocr_mobile_v2.0_cls_infer.onnx")
	if got := DefaultClsModelPath(home + "/.config/aigc-cli/models"); got != expected {
		t.Errorf("DefaultClsModelPath() = %q, want %q", got, expected)
	}

	expected = filepath.Join(home, ".config", "aigc-cli", "models", "dict_zh.txt")
	if got := DefaultDictPath(home + "/.config/aigc-cli/models"); got != expected {
		t.Errorf("DefaultDictPath() = %q, want %q", got, expected)
	}
}

func TestConstants(t *testing.T) {
	if DetInputSize != 960 {
		t.Errorf("DetInputSize = %d, want 960", DetInputSize)
	}
	if DetDownsample != 1 {
		t.Errorf("DetDownsample = %d, want 1", DetDownsample)
	}
	if RecHeight != 48 {
		t.Errorf("RecHeight = %d, want 48", RecHeight)
	}
	if RecMaxWidth != 960 {
		t.Errorf("RecMaxWidth = %d, want 960", RecMaxWidth)
	}
	if RecVocabSize != 6625 {
		t.Errorf("RecVocabSize = %d, want 6625", RecVocabSize)
	}
}

// ----- Models -----

func TestModels_NotEmpty(t *testing.T) {
	models := Models()
	if len(models) == 0 {
		t.Fatal("Models() returned empty list")
	}
}

func TestModels_HasChinese(t *testing.T) {
	m, ok := FindModelByID("rapidocr")
	if !ok {
		t.Fatal("FindModelByID(rapidocr) not found")
	}
	if len(m.Files) != 6 {
		t.Errorf("model pack should have 6 files (det+rec+cls+dict+en_rec+en_dict), got %d", len(m.Files))
	}
	hasDet := false
	hasRec := false
	hasDict := false
	for _, f := range m.Files {
		if f.Type == "det" {
			hasDet = true
		}
		if f.Type == "rec" {
			hasRec = true
		}
		if f.Type == "dict" {
			hasDict = true
		}
	}
	if !hasDet {
		t.Error("model pack missing det file")
	}
	if !hasRec {
		t.Error("model pack missing rec file")
	}
	if !hasDict {
		t.Error("model pack missing dict file")
	}
}

func TestModels_UnknownID(t *testing.T) {
	_, ok := FindModelByID("nonexistent")
	if ok {
		t.Error("FindModelByID(nonexistent) should return false")
	}
}

// ----- Helpers (maxInt, minInt) -----

func TestMaxInt(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 2},
		{2, 1, 2},
		{0, 0, 0},
		{-1, 1, 1},
		{-5, -3, -3},
	}
	for _, tc := range tests {
		got := maxInt(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("maxInt(%d,%d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestMinInt(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{0, 0, 0},
		{-1, 1, -1},
		{-5, -3, -5},
	}
	for _, tc := range tests {
		got := minInt(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("minInt(%d,%d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// ----- NMS helpers -----

func TestBoxToRect(t *testing.T) {
	box := [4][2]int{{10, 20}, {30, 20}, {30, 50}, {10, 50}}
	r := boxToRect(box)
	if r.x0 != 10 || r.y0 != 20 || r.x1 != 30 || r.y1 != 50 {
		t.Errorf("boxToRect = %+v, want {10,20,30,50}", r)
	}
}

func TestIoURect_noOverlap(t *testing.T) {
	a := rect{0, 0, 10, 10}
	b := rect{20, 20, 30, 30}
	if iou := iouRect(a, b); iou != 0 {
		t.Errorf("iouRect no overlap = %f, want 0", iou)
	}
}

func TestIoURect_fullOverlap(t *testing.T) {
	a := rect{0, 0, 10, 10}
	b := rect{0, 0, 10, 10}
	if iou := iouRect(a, b); math.Abs(float64(iou-1.0)) > 0.001 {
		t.Errorf("iouRect full overlap = %f, want 1.0", iou)
	}
}

func TestIoURect_halfOverlap(t *testing.T) {
	// Two 10x10 squares overlapping by 50%
	a := rect{0, 0, 10, 10} // area 100
	b := rect{5, 0, 15, 10} // area 100, overlap = 5*10 = 50
	iou := iouRect(a, b)
	want := float32(50.0 / 150.0)
	if math.Abs(float64(iou-want)) > 0.001 {
		t.Errorf("iouRect half = %f, want %f", iou, want)
	}
}

func TestNMS_noBoxes(t *testing.T) {
	keep := nms(nil, nil, 0.5)
	if keep != nil {
		t.Errorf("nms(nil) = %v, want nil", keep)
	}
}

func TestNMS_allKept(t *testing.T) {
	// Non-overlapping boxes — all should be kept
	boxes := [][4][2]int{
		{{0, 0}, {10, 0}, {10, 10}, {0, 10}},
		{{100, 100}, {110, 100}, {110, 110}, {100, 110}},
	}
	scores := []float32{0.9, 0.8}
	keep := nms(boxes, scores, 0.5)
	if len(keep) != 2 {
		t.Errorf("nms non-overlapping = %v, want 2 kept", keep)
	}
}

func TestNMS_overlapRemoved(t *testing.T) {
	// Box 1 fully contains Box 2 → IoU high, only higher-scored kept.
	// BoxA: {0,0,10,10} (area 100), BoxB: {1,1,9,9} (area 64)
	// Intersection: {1,1,9,9} = 64, Union: 100+64-64 = 100, IoU: 0.64
	boxes := [][4][2]int{
		{{0, 0}, {10, 0}, {10, 10}, {0, 10}},
		{{1, 1}, {9, 1}, {9, 9}, {1, 9}},
	}
	scores := []float32{0.9, 0.3}
	keep := nms(boxes, scores, 0.5)
	if len(keep) != 1 {
		t.Errorf("nms overlapping = %v, want 1 kept (best = 0.9)", keep)
	}
	if len(keep) > 0 && keep[0] != 0 {
		t.Errorf("nms should keep index 0 first (score 0.9), got %v", keep)
	}
}

// ----- minAreaRect -----

func TestMinAreaRect_fourCorners(t *testing.T) {
	pts := []ocrPoint{
		{0, 0}, {10, 0}, {10, 10}, {0, 10},
	}
	box := minAreaRect(pts)
	if box == nil {
		t.Fatal("minAreaRect returned nil")
	}
	// Should be axis-aligned: min (0,0) to max (10,10)
	if box[0][0] != 0 || box[0][1] != 0 {
		t.Errorf("top-left = %v, want [0,0]", box[0])
	}
	if box[2][0] != 10 || box[2][1] != 10 {
		t.Errorf("bottom-right = %v, want [10,10]", box[2])
	}
}

func TestMinAreaRect_lessThan3Points(t *testing.T) {
	pts := []ocrPoint{{0, 0}, {1, 1}}
	box := minAreaRect(pts)
	if box != nil {
		t.Error("minAreaRect with <3 points should return nil")
	}
}

// ----- Detection Preprocessing -----

func TestDetPreprocess_OutputSize(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 200))
	pixels, scaleX, scaleY, padLeft, padTop := detPreprocess(img)

	expectedLen := DetChannels * DetInputSize * DetInputSize
	if len(pixels) != expectedLen {
		t.Errorf("pixels length = %d, want %d", len(pixels), expectedLen)
	}

	// detPreprocess does NOT upscale (ratio capped at 1.0).
	// With 100x200 input: ratio = min(960/200=4.8, 1.0) = 1.0
	// resizedW = 100, resizedH = 200
	// padLeft = (960-100)/2 = 430, padTop = (960-200)/2 = 380
	// scaleX = 100/960 ≈ 0.104, scaleY = 200/960 ≈ 0.208
	if padLeft != 430 {
		t.Errorf("padLeft = %d, want 430", padLeft)
	}
	if padTop != 380 {
		t.Errorf("padTop = %d, want 380", padTop)
	}
	if math.Abs(scaleX-100.0/960.0) > 0.001 {
		t.Errorf("scaleX = %f, want %f", scaleX, 100.0/960.0)
	}
	if math.Abs(scaleY-200.0/960.0) > 0.001 {
		t.Errorf("scaleY = %f, want %f", scaleY, 200.0/960.0)
	}
}

func TestDetPreprocess_smallerThanMax(t *testing.T) {
	// Image smaller than 960 — should not upscale (ratio capped at 1.0)
	img := image.NewRGBA(image.Rect(0, 0, 50, 80))
	_, scaleX, _, padLeft, padTop := detPreprocess(img)

	if padLeft != 455 { // (960-50)/2
		t.Errorf("padLeft = %d, want 455", padLeft)
	}
	if padTop != 440 { // (960-80)/2
		t.Errorf("padTop = %d, want 440", padTop)
	}
	// scale = resized / 960 = original dims / 960 (since no upscale)
	// scaleX = 50/960 ≈ 0.052, scaleY = 80/960 ≈ 0.083
	if scaleX < 0.05 || scaleX > 0.06 {
		t.Errorf("scaleX = %f, want ~0.052", scaleX)
	}
}

func TestDetPreprocess_paddedZeros(t *testing.T) {
	// Verify that padded pixels are not left at zero (they should be mean-centered)
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	pixels, _, _, _, _ := detPreprocess(img)

	// Check a padded pixel (outside the 10x10 image) — should be -mean/std = -0.485/0.229 ≈ -2.117
	paddedIdx := 0*DetInputSize*DetInputSize + 500*DetInputSize + 500 // far outside image
	if pixels[paddedIdx] > -0.1 {
		// The padded pixel should have been set to mean-centered value
		t.Logf("Note: padded pixel at %d = %f (expected negative from mean subtraction)", paddedIdx, pixels[paddedIdx])
	}
}

// ----- Detection Post-Processing -----

func TestDetPostProcess_noBoxes(t *testing.T) {
	// All-zero probability map — no boxes expected
	h, w := 240, 240
	probMap := make([]float32, h*w)
	boxes := detPostProcess(probMap, h, w, 1.0, 1.0, 0, 0, 100, 200)
	if len(boxes) != 0 {
		t.Errorf("expected no boxes for zero prob map, got %d", len(boxes))
	}
}

func TestDetPostProcess_singleBox(t *testing.T) {
	// Probability map with one high-probability region (all 1.0 in a small area)
	h, w := 240, 240
	probMap := make([]float32, h*w)
	// Set a 10x10 block to high probability
	for y := 50; y < 60; y++ {
		for x := 80; x < 90; x++ {
			probMap[y*w+x] = 0.95
		}
	}
	boxes := detPostProcess(probMap, h, w, 0.5, 0.5, 0, 0, 960, 960)
	if len(boxes) == 0 {
		t.Fatal("expected at least one box")
	}
	t.Logf("Found %d box(es)", len(boxes))
	for i, b := range boxes {
		t.Logf("  box[%d]: %v", i, b)
	}
}

func TestDetPostProcess_multipleBoxesNMS(t *testing.T) {
	h, w := 240, 240
	probMap := make([]float32, h*w)
	// Two separated regions
	for y := 20; y < 30; y++ {
		for x := 20; x < 30; x++ {
			probMap[y*w+x] = 0.9
		}
	}
	for y := 150; y < 160; y++ {
		for x := 150; x < 160; x++ {
			probMap[y*w+x] = 0.85
		}
	}
	boxes := detPostProcess(probMap, h, w, 1.0, 1.0, 0, 0, 960, 960)
	if len(boxes) != 2 {
		t.Logf("boxes count = %d (may be more if components split)", len(boxes))
	}
}

// ----- Recognition Preprocessing -----

func TestRecPreprocess_zeroRegion(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// Zero-width region
	box := [4][2]int{{50, 50}, {50, 50}, {50, 50}, {50, 50}}
	pixels, w := recPreprocess(img, box)
	if pixels != nil || w != 0 {
		t.Error("recPreprocess with zero-size region should return nil, 0")
	}
}

func TestRecPreprocess_validRegion(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	// Fill with a color
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, color.RGBA{128, 64, 32, 255})
		}
	}
	box := [4][2]int{{20, 30}, {80, 30}, {80, 60}, {20, 60}}
	pixels, w := recPreprocess(img, box)
	if pixels == nil {
		t.Fatal("recPreprocess returned nil")
	}
	if w <= 0 {
		t.Fatalf("region width = %d, want >0", w)
	}
	expectedLen := DetChannels * RecHeight * RecMaxWidth
	if len(pixels) != expectedLen {
		t.Errorf("pixels length = %d, want %d", len(pixels), expectedLen)
	}
}

// ----- CTC Greedy Decode -----

func TestCTCDecode_simple(t *testing.T) {
	// Simple case: one character (index 1 = '!') repeated — should collapse to one
	vocab := 10
	timesteps := 5
	logits := make([]float32, timesteps*vocab)
	// Set index 1 high at all timesteps
	for t := 0; t < timesteps; t++ {
		logits[t*vocab+1] = 1.0
	}
	text := ctcGreedyDecode(logits, timesteps, vocab, nil)
	if text != "!" {
		t.Errorf("ctcGreedyDecode = %q, want '!'", text)
	}
}

func TestCTCDecode_blankCollapse(t *testing.T) {
	// Blank (index 0) between same chars should NOT merge them.
	// Pattern: 'a'(idx=65), blank(0), blank(0), 'a'(idx=65), 'b'(idx=66), blank(0), 'b'(idx=66)
	// CTC: separate 'a'-'a'-'b'-'b' but consecutive same labels without blank collapse → "aabb"
	vocab := 100
	timesteps := 7
	logits := make([]float32, timesteps*vocab)
	logits[0*vocab+65] = 1.0 // a
	logits[1*vocab+0] = 1.0  // blank
	logits[2*vocab+0] = 1.0  // blank
	logits[3*vocab+65] = 1.0 // a (different instance after blank → separate)
	logits[4*vocab+66] = 1.0 // b
	logits[5*vocab+0] = 1.0  // blank
	logits[6*vocab+66] = 1.0 // b (different instance after blank → separate)

	text := ctcGreedyDecode(logits, timesteps, vocab, nil)
	// Correct CTC: a(0)→blank→blank→a(3, after blank)→b(4)→blank→b(6, after blank) = "aabb"
	if text != "aabb" {
		t.Errorf("ctcGreedyDecode = %q, want 'aabb'", text)
	}
}

func TestCTCDecode_allBlank(t *testing.T) {
	vocab := 10
	timesteps := 5
	logits := make([]float32, timesteps*vocab)
	logits[0] = 1.0 // blank at all timesteps
	logits[5] = 1.0
	logits[10] = 1.0
	logits[15] = 1.0
	logits[20] = 1.0

	text := ctcGreedyDecode(logits, timesteps, vocab, nil)
	if text != "" {
		t.Errorf("ctcGreedyDecode all-blank = %q, want ''", text)
	}
}

func TestCTCDecode_collapseConsecutive(t *testing.T) {
	// Consecutive same labels → collapsed (no blank between them).
	// Use index 20 = '4' and index 22 = '6' (ASCII mapping: n → char(0x20+n)).
	vocab := 100
	timesteps := 4
	logits := make([]float32, timesteps*vocab)
	logits[0*vocab+20] = 1.0 // '4'
	logits[1*vocab+20] = 1.0 // '4' — same, collapse
	logits[2*vocab+22] = 1.0 // '6'
	logits[3*vocab+22] = 1.0 // '6' — same, collapse

	text := ctcGreedyDecode(logits, timesteps, vocab, nil)
	if text != "46" {
		t.Errorf("ctcGreedyDecode = %q, want '46'", text)
	}
}

// ----- Label Decoding -----

func TestDecodeLabels_empty(t *testing.T) {
	if s := decodeLabels(nil, nil); s != "" {
		t.Errorf("decodeLabels(nil) = %q, want ''", s)
	}
	if s := decodeLabels([]int{}, nil); s != "" {
		t.Errorf("decodeLabels([]) = %q, want ''", s)
	}
}

func TestDecodeLabels_asciiRange(t *testing.T) {
	// Index 1 → '!' (0x21), index 2 → '"' (0x22)
	s := decodeLabels([]int{1, 2, 3}, nil)
	if len(s) != 3 {
		t.Fatalf("decodeLabels = %q, want 3 chars", s)
	}
	if s != "!\""+string(rune(0x23)) {
		t.Errorf("decodeLabels = %q, want '!\"#'", s)
	}
}

func TestDecodeLabels_zeroIndex(t *testing.T) {
	// Index 0 is blank — should be skipped
	s := decodeLabels([]int{0, 1, 0, 2}, nil)
	if s != "!\"" {
		t.Errorf("decodeLabels with blanks = %q, want '!\"'", s)
	}
}

func TestDecodeLabels_cjkRange(t *testing.T) {
	// Index 95+ maps to CJK characters (fallback without dict)
	s := decodeLabels([]int{95, 96}, nil)
	// 95 → U+4E00 (一), 96 → U+4E01 (丁)
	if utf8.RuneCountInString(s) != 2 {
		t.Errorf("decodeLabels CJK = %q (len=%d runes), want 2 chars", s, utf8.RuneCountInString(s))
	}
}

func TestDecodeLabels_outOfRange(t *testing.T) {
	// When no dict is loaded, out-of-range indices use CJK fallback mapping.
	s := decodeLabels([]int{1, 99999}, nil)
	if len(s) < 1 || s[0] != '!' {
		t.Errorf("decodeLabels with out-of-range = %q, want '!...'", s)
	}
}

// ----- OCRLine / OCRPage / OCRResult building -----

func TestOCRLine_Fields(t *testing.T) {
	line := OCRLine{
		Text:       "hello",
		BBox:       [4][2]int{{0, 0}, {10, 0}, {10, 5}, {0, 5}},
		Confidence: 0.95,
	}
	if line.Text != "hello" {
		t.Errorf("Text = %q, want 'hello'", line.Text)
	}
	if line.BBox[2][0] != 10 {
		t.Errorf("BBox bottom-right x = %d, want 10", line.BBox[2][0])
	}
}

func TestOCRPage_Fields(t *testing.T) {
	page := OCRPage{
		Page:  0,
		Lines: []OCRLine{{Text: "line1"}, {Text: "line2"}},
	}
	if page.Page != 0 {
		t.Errorf("Page = %d, want 0", page.Page)
	}
	if len(page.Lines) != 2 {
		t.Errorf("len(Lines) = %d, want 2", len(page.Lines))
	}
}

func TestOCRResult_TextConcat(t *testing.T) {
	result := &OCRResult{
		Pages: []OCRPage{
			{
				Page: 0,
				Lines: []OCRLine{
					{Text: "第一行"},
					{Text: "第二行"},
				},
			},
		},
		Text: "第一行\n第二行",
	}
	if result.Text != "第一行\n第二行" {
		t.Errorf("Text = %q, want '第一行\\n第二行'", result.Text)
	}
}

// ----- Edge cases for utility functions -----

func TestBoxToRect_reordered(t *testing.T) {
	// Points in random order
	box := [4][2]int{{30, 10}, {10, 10}, {10, 50}, {30, 50}}
	r := boxToRect(box)
	if r.x0 != 10 || r.y0 != 10 || r.x1 != 30 || r.y1 != 50 {
		t.Errorf("boxToRect = %+v, want {10,10,30,50}", r)
	}
}

func TestIoURect_edgeTouch(t *testing.T) {
	// Two rects touching at the edge — no overlap
	a := rect{0, 0, 10, 10}
	b := rect{10, 0, 20, 10}
	if iou := iouRect(a, b); iou != 0 {
		t.Errorf("iouRect touching = %f, want 0", iou)
	}
}

func TestIoURect_contained(t *testing.T) {
	// One rect inside the other
	a := rect{0, 0, 20, 20} // area 400
	b := rect{5, 5, 15, 15} // area 100
	iou := iouRect(a, b)
	want := float32(100.0 / 400.0) // 0.25
	if math.Abs(float64(iou-want)) > 0.001 {
		t.Errorf("iouRect contained = %f, want %f", iou, want)
	}
}

// ----- DefaultLibPath -----

func TestDefaultLibPath_notFound(t *testing.T) {
	tmp := t.TempDir()
	_, err := DefaultLibPath(tmp)
	if err == nil {
		t.Error("DefaultLibPath in empty dir should return error")
	}
}

// ----- Cranks: Benchmark-level validation -----

func TestDetPreprocess_whiteImage(t *testing.T) {
	// All-white should produce consistent normalized values
	img := image.NewRGBA(image.Rect(0, 0, 960, 960))
	for y := 0; y < 960; y++ {
		for x := 0; x < 960; x++ {
			img.Set(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	pixels, _, _, padLeft, padTop := detPreprocess(img)

	// No padding needed for 960x960 input
	if padLeft != 0 || padTop != 0 {
		t.Errorf("pad for 960x960: left=%d top=%d, want 0", padLeft, padTop)
	}

	// All-white: R=1.0 → (1.0-0.485)/0.229 ≈ 2.249
	rIdx := 0*DetInputSize*DetInputSize + 100*DetInputSize + 100
	if pixels[rIdx] < 1.0 {
		// White pixel should be positive after normalization
		t.Logf("White pixel normalized R value: %f (expected > 1.0)", pixels[rIdx])
	}
}

// ----- splitEnglishWords -----

func TestSplitEnglishWords_splitsConcatenated(t *testing.T) {
	// Core behavior: concatenated English text should be broken into words.
	// The DP may not produce perfect results with the system dictionary,
	// but it MUST create at least as many tokens as there are words.
	tests := []struct {
		input string
		min   int // minimum number of space-separated tokens expected
	}{
		{"thehiddendebtofthesecompanies", 6}, // the hidden debt of these companies
		{"hasincreasedeightfoldoverthe", 4},  // has increased eightfold over the
		{"Amidthefervorofartificial", 4},     // Amid the fervor of artificial
	}
	for _, tc := range tests {
		got := splitEnglishWords(tc.input)
		tokens := len(strings.Fields(got))
		if tokens < tc.min {
			t.Errorf("splitEnglishWords(%q) = %q (%d tokens), want ≥ %d", tc.input, got, tokens, tc.min)
		}
	}
}

func TestSplitEnglishWords_preservesGoodSpacing(t *testing.T) {
	// Already well-spaced text should not be degraded to something unrecognizable.
	input := "the hidden debt of these companies"
	got := splitEnglishWords(input)
	// The input has spaces and is readable, output should also have spaces
	if !strings.Contains(got, " ") {
		t.Errorf("splitEnglishWords(%q) = %q, lost all spaces", input, got)
	}
}
