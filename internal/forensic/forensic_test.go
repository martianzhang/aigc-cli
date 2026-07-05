package forensic

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// ── scorer.go tests ────────────────────────────────────────────────────────

func TestScoreToLevel(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  Level
	}{
		{"confirmed high", 0.95, LevelConfirmedAI},
		{"confirmed edge", 0.90, LevelConfirmedAI},
		{"likely ai upper", 0.89, LevelLikelyAI},
		{"likely ai mid", 0.75, LevelLikelyAI},
		{"likely ai edge", 0.65, LevelLikelyAI},
		{"suspicious upper", 0.64, LevelSuspicious},
		{"suspicious mid", 0.50, LevelSuspicious},
		{"suspicious edge", 0.40, LevelSuspicious},
		{"low upper", 0.39, LevelLow},
		{"low mid", 0.30, LevelLow},
		{"low edge", 0.20, LevelLow},
		{"human upper", 0.19, LevelHuman},
		{"human zero", 0.0, LevelHuman},
		{"human negative", -0.1, LevelHuman},
		{"human", 0.10, LevelHuman},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreToLevel(tt.score)
			if got != tt.want {
				t.Errorf("scoreToLevel(%v) = %v, want %v", tt.score, got, tt.want)
			}
		})
	}
}

func TestLevelEmoji(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelHuman, "🟢"},
		{LevelLow, "🟡"},
		{LevelSuspicious, "🟠"},
		{LevelLikelyAI, "🔴"},
		{LevelConfirmedAI, "🤖"},
		{Level(-1), "⚪"},
		{Level(99), "⚪"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Level_%d", tt.level), func(t *testing.T) {
			got := levelEmoji(tt.level)
			if got != tt.want {
				t.Errorf("levelEmoji(%v) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestLevelSummary(t *testing.T) {
	tests := []struct {
		level Level
		rate  float64
		check []string // substrings to check
	}{
		{LevelHuman, 0.05, []string{"🟢", "5%", "human"}},
		{LevelHuman, 0.19, []string{"🟢", "19%", "human"}},
		{LevelLow, 0.35, []string{"🟡", "35%", "suspicious"}},
		{LevelSuspicious, 0.55, []string{"🟠", "55%", "AI"}},
		{LevelLikelyAI, 0.80, []string{"🔴", "80%", "AI"}},
		{LevelConfirmedAI, 0.99, []string{"🤖", "99%", "Confirmed"}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("Level_%d", tt.level), func(t *testing.T) {
			got := levelSummary(tt.level, tt.rate)
			for _, s := range tt.check {
				if !contains(got, s) {
					t.Errorf("levelSummary(%v, %v) = %q, want it to contain %q", tt.level, tt.rate, got, s)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestAnalyze_C2PASaturates(t *testing.T) {
	opts := Options{
		C2PAPresent: true,
		C2PAVendor:  "OpenAI",
		C2PASource:  "AI Generated",
	}
	r := Analyze(opts)
	if r.AIGenRate != 0.99 {
		t.Errorf("C2PA: expected AIGenRate=0.99, got %v", r.AIGenRate)
	}
	if r.Level != LevelConfirmedAI {
		t.Errorf("C2PA: expected LevelConfirmedAI, got %v", r.Level)
	}
}

func TestAnalyze_TC260Saturates(t *testing.T) {
	opts := Options{
		TC260Present:  true,
		TC260Provider: "test-provider",
	}
	r := Analyze(opts)
	if r.AIGenRate != 0.99 {
		t.Errorf("TC260: expected AIGenRate=0.99, got %v", r.AIGenRate)
	}
	if r.Level != LevelConfirmedAI {
		t.Errorf("TC260: expected LevelConfirmedAI, got %v", r.Level)
	}
}

func TestAnalyze_SynthID(t *testing.T) {
	opts := Options{
		SynthIDPresent: true,
		SynthIDLikely:  true,
		SynthIDSource:  "Google DeepMind",
	}
	r := Analyze(opts)
	if r.AIGenRate <= 0 {
		t.Errorf("SynthID: expected positive AIGenRate, got %v", r.AIGenRate)
	}
	if len(r.Signals) == 0 {
		t.Error("SynthID: expected at least 1 signal")
	}
	// Verify SynthID signal is present
	found := false
	for _, s := range r.Signals {
		if s.Name == "SynthID Watermark" {
			found = true
			break
		}
	}
	if !found {
		t.Error("SynthID: expected 'SynthID Watermark' signal")
	}
}

func TestAnalyze_CameraEXIF(t *testing.T) {
	opts := Options{
		CameraPresent: true,
		CameraMake:    "Canon",
		CameraModel:   "EOS R5",
	}
	r := Analyze(opts)
	// Camera reduces AI score
	if r.AIGenRate > 0.5 {
		t.Errorf("Camera EXIF: expected AIGenRate <= 0.5 (camera = human signal), got %v", r.AIGenRate)
	}
}

func TestAnalyze_NoCameraEXIF(t *testing.T) {
	opts := Options{}
	r := Analyze(opts)
	// No camera should produce "No Camera EXIF" signal
	found := false
	for _, s := range r.Signals {
		if s.Name == "No Camera EXIF" {
			found = true
			break
		}
	}
	if !found {
		t.Error("No Camera EXIF signal not found in results")
	}
}

func TestAnalyze_AllSignals(t *testing.T) {
	opts := Options{
		SynthIDPresent: true,
		SynthIDLikely:  true,
		ONNXScore:      0.85,
		ONNXModelSize:  "large",
		FFTScore:       0.70,
		NoiseScore:     0.60,
		JPEGScore:      0.45,
	}
	r := Analyze(opts)
	if r.AIGenRate <= 0 {
		t.Errorf("all signals: expected positive AIGenRate, got %v", r.AIGenRate)
	}
	if len(r.Signals) < 4 {
		t.Errorf("all signals: expected >=4 signals, got %d: %v", len(r.Signals), r.Signals)
	}

	// Verify all named signals present
	names := map[string]bool{}
	for _, s := range r.Signals {
		names[s.Name] = true
	}
	for _, want := range []string{"SynthID Watermark", "No Camera EXIF", "AI Model", "FFT Spectral", "Noise Residual", "JPEG Analysis"} {
		if !names[want] {
			t.Errorf("all signals: missing signal %q", want)
		}
	}
}

func TestAnalyze_NegativeScoresExcluded(t *testing.T) {
	opts := Options{
		ONNXScore:  -1,
		FFTScore:   -1,
		NoiseScore: -1,
		JPEGScore:  -1,
	}
	r := Analyze(opts)
	for _, s := range r.Signals {
		if s.Name == "AI Model" || s.Name == "FFT Spectral" || s.Name == "Noise Residual" || s.Name == "JPEG Analysis" {
			t.Errorf("negative score not excluded: signal %q has score %v", s.Name, s.Score)
		}
	}
}

func TestAnalyze_NilOptions(t *testing.T) {
	opts := Options{}
	r := Analyze(opts)
	if r == nil {
		t.Fatal("Analyze returned nil")
	}
	if r.Emoji == "" {
		t.Errorf("empty options: expected non-empty emoji, got %q", r.Emoji)
	}
	// No signals beyond EXIF → level depends on whether CameraPresent is set
}

// ── FFT tests ──────────────────────────────────────────────────────────────

func newUniformGray(w, h int, grayVal uint8) image.Image {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetGray(x, y, color.Gray{Y: grayVal})
		}
	}
	return img
}

func newSyntheticImage(w, h int, fn func(x, y int) uint8) image.Image {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetGray(x, y, color.Gray{Y: fn(x, y)})
		}
	}
	return img
}

func TestAnalyzeFFT_TooSmall(t *testing.T) {
	img := newUniformGray(32, 32, 128)
	score := AnalyzeFFT(img)
	if score != -1 {
		t.Errorf("32x32 image: expected -1 (too small), got %v", score)
	}
}

func TestAnalyzeFFT_UniformImage(t *testing.T) {
	// Uniform gray = zero-frequency dominant → flat spectrum
	img := newUniformGray(128, 128, 128)
	score := AnalyzeFFT(img)
	if score < 0 || score > 1 {
		t.Errorf("uniform image: expected 0-1, got %v", score)
	}
	// Uniform image has only DC component → HF ratio is near 0
	// So score should be low (natural-like)
	if score > 0.6 {
		t.Errorf("uniform image: expected low score (flat spectrum), got %v", score)
	}
}

func TestAnalyzeFFT_NoiseImage(t *testing.T) {
	// Random noise = flat spectrum → should have higher HF energy
	rng := newRNG(42)
	img := newSyntheticImage(128, 128, func(x, y int) uint8 {
		return uint8(rng.next() % 256)
	})
	score := AnalyzeFFT(img)
	if score < 0 || score > 1 {
		t.Errorf("noise image: expected 0-1, got %v", score)
	}
	// White noise has lots of high-frequency content
	if score < 0.1 {
		t.Errorf("noise image: expected non-trivial score, got %v", score)
	}
}

func TestAnalyzeFFT_GradientImage(t *testing.T) {
	// Smooth gradient = low frequency dominant → lower score
	img := newSyntheticImage(128, 128, func(x, y int) uint8 {
		return uint8((x + y) * 255 / 256)
	})
	score := AnalyzeFFT(img)
	if score < 0 || score > 1 {
		t.Errorf("gradient image: expected 0-1, got %v", score)
	}
}

func TestAnalyzeFFT_LargeImageDownsample(t *testing.T) {
	// Image > 512 in one dimension triggers downsampling path
	img := newUniformGray(800, 600, 100)
	score := AnalyzeFFT(img)
	if score < 0 || score > 1 {
		t.Errorf("large image: expected 0-1, got %v", score)
	}
}

func TestAnalyzeFFT_ExactEdgeSize(t *testing.T) {
	// Exactly 512px should NOT trigger downsampling
	img := newUniformGray(512, 512, 100)
	score := AnalyzeFFT(img)
	if score < 0 || score > 1 {
		t.Errorf("512x512 image: expected 0-1, got %v", score)
	}
}

func TestAnalyzeFFT_RGBAImage(t *testing.T) {
	// Non-gray image (RGBA) should also work
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	score := AnalyzeFFT(img)
	if score < 0 || score > 1 {
		t.Errorf("RGBA image: expected 0-1, got %v", score)
	}
}

// ── Noise residual tests ───────────────────────────────────────────────────

func TestAnalyzeNoise_TooSmall(t *testing.T) {
	img := newUniformGray(8, 8, 128)
	score := AnalyzeNoiseResidual(img)
	if score != -1 {
		t.Errorf("8x8 image: expected -1 (too small), got %v", score)
	}
}

func TestAnalyzeNoise_UniformImage(t *testing.T) {
	img := newUniformGray(64, 64, 128)
	score := AnalyzeNoiseResidual(img)
	if score < 0 || score > 1 {
		t.Errorf("uniform image: expected 0-1, got %v", score)
	}
}

func TestAnalyzeNoise_RandomImage(t *testing.T) {
	rng := newRNG(42)
	img := newSyntheticImage(64, 64, func(x, y int) uint8 {
		return uint8(rng.next() % 256)
	})
	score := AnalyzeNoiseResidual(img)
	if score < 0 || score > 1 {
		t.Errorf("random image: expected 0-1, got %v", score)
	}
}

func TestAnalyzeNoise_LargeImageDownsample(t *testing.T) {
	img := newUniformGray(2048, 2048, 100)
	score := AnalyzeNoiseResidual(img)
	if score < 0 || score > 1 {
		t.Errorf("2048x2048 image: expected 0-1, got %v", score)
	}
}

// ── JPEG tests ─────────────────────────────────────────────────────────────

func writeTestJPEG(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test JPEG: %v", err)
	}
	return path
}

func TestAnalyzeJPEG_NotJPEG(t *testing.T) {
	path := writeTestJPEG(t, []byte{0x89, 0x50, 0x4E, 0x47}) // PNG header
	score := AnalyzeJPEGDoubleQuant(path)
	if score != -1 {
		t.Errorf("PNG file: expected -1, got %v", score)
	}
}

func TestAnalyzeJPEG_StandardTables(t *testing.T) {
	// Build minimal JPEG with SOI + standard DQT + SOS
	// SOI marker
	jpeg := []byte{0xFF, 0xD8}
	// DQT marker
	jpeg = append(jpeg, 0xFF, 0xDB)
	// Length: 2 (self) + 65 (precision+table) = 67 = 0x0043
	jpeg = append(jpeg, 0x00, 0x43)
	// Precision=0 (8-bit), table ID=0
	jpeg = append(jpeg, 0x00)
	// Standard luminance table
	stdLum := []byte{
		16, 11, 10, 16, 24, 40, 51, 61,
		12, 12, 14, 19, 26, 58, 60, 55,
		14, 13, 16, 24, 40, 57, 69, 56,
		14, 17, 22, 29, 51, 87, 80, 62,
		18, 22, 37, 56, 68, 109, 103, 77,
		24, 35, 55, 64, 81, 104, 113, 92,
		49, 64, 78, 87, 103, 121, 120, 101,
		72, 92, 95, 98, 112, 100, 103, 99,
	}
	jpeg = append(jpeg, stdLum...)
	// SOS to stop parsing
	jpeg = append(jpeg, 0xFF, 0xDA)
	// Minimal scan data
	jpeg = append(jpeg, 0x00, 0x08, 0x3F, 0x00)

	path := writeTestJPEG(t, jpeg)
	score := AnalyzeJPEGDoubleQuant(path)
	if score < 0 || score > 1 {
		t.Errorf("standard JPEG: expected 0-1, got %v", score)
	}
	// Standard tables should not be flagged as non-standard
	// Single set of standard tables → score 0.45
	if score > 0.5 {
		t.Errorf("standard JPEG: expected <= 0.5, got %v", score)
	}
}

func TestAnalyzeJPEG_NonStandardTables(t *testing.T) {
	// JPEG with clearly non-standard quantization table
	jpeg := []byte{0xFF, 0xD8}
	// DQT marker
	jpeg = append(jpeg, 0xFF, 0xDB)
	// Length
	jpeg = append(jpeg, 0x00, 0x43)
	// Precision=0, table ID=0
	jpeg = append(jpeg, 0x00)
	// Non-standard table: all 1s (would never be standard)
	nonStd := make([]byte, 64)
	for i := range nonStd {
		nonStd[i] = 1
	}
	jpeg = append(jpeg, nonStd...)
	// SOS
	jpeg = append(jpeg, 0xFF, 0xDA)
	jpeg = append(jpeg, 0x00, 0x08, 0x3F, 0x00)

	path := writeTestJPEG(t, jpeg)
	score := AnalyzeJPEGDoubleQuant(path)
	if score < 0 || score > 1 {
		t.Errorf("non-standard JPEG: expected 0-1, got %v", score)
	}
	// Non-standard table → score 0.65
	if score < 0.5 {
		t.Errorf("non-standard JPEG: expected >= 0.5 (non-standard table), got %v", score)
	}
}

func TestAnalyzeJPEG_MultipleDQTs(t *testing.T) {
	// JPEG with multiple DQT markers = double compression history
	jpeg := []byte{0xFF, 0xD8}

	// DQT #1: standard luminance
	jpeg = append(jpeg, 0xFF, 0xDB, 0x00, 0x43, 0x00)
	stdLum := []byte{
		16, 11, 10, 16, 24, 40, 51, 61,
		12, 12, 14, 19, 26, 58, 60, 55,
		14, 13, 16, 24, 40, 57, 69, 56,
		14, 17, 22, 29, 51, 87, 80, 62,
		18, 22, 37, 56, 68, 109, 103, 77,
		24, 35, 55, 64, 81, 104, 113, 92,
		49, 64, 78, 87, 103, 121, 120, 101,
		72, 92, 95, 98, 112, 100, 103, 99,
	}
	jpeg = append(jpeg, stdLum...)

	// DQT #2: standard chrominance
	jpeg = append(jpeg, 0xFF, 0xDB, 0x00, 0x43, 0x01) // table ID=1
	stdChr := []byte{
		17, 18, 24, 47, 99, 99, 99, 99,
		18, 21, 26, 66, 99, 99, 99, 99,
		24, 26, 56, 99, 99, 99, 99, 99,
		47, 66, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
	}
	jpeg = append(jpeg, stdChr...)

	// SOS
	jpeg = append(jpeg, 0xFF, 0xDA, 0x00, 0x08, 0x3F, 0x00)

	path := writeTestJPEG(t, jpeg)
	score := AnalyzeJPEGDoubleQuant(path)
	if score < 0 || score > 1 {
		t.Errorf("multi-DQT JPEG: expected 0-1, got %v", score)
	}
	// 2 standard DQTs should give score 0.45 (single-set)
}

func TestAnalyzeJPEG_EmptyFile(t *testing.T) {
	path := writeTestJPEG(t, []byte{})
	score := AnalyzeJPEGDoubleQuant(path)
	if score != -1 {
		t.Errorf("empty file: expected -1, got %v", score)
	}
}

func TestAnalyzeJPEG_NotFound(t *testing.T) {
	score := AnalyzeJPEGDoubleQuant("/nonexistent/test.jpg")
	if score != -1 {
		t.Errorf("nonexistent file: expected -1, got %v", score)
	}
}

// ── isStandardTable tests (tested indirectly via AnalyzeJPEGDoubleQuant) ────
// But we can also access it since it's in the same package

func TestIsStandardTable_Luminance(t *testing.T) {
	table := [64]byte{
		16, 11, 10, 16, 24, 40, 51, 61,
		12, 12, 14, 19, 26, 58, 60, 55,
		14, 13, 16, 24, 40, 57, 69, 56,
		14, 17, 22, 29, 51, 87, 80, 62,
		18, 22, 37, 56, 68, 109, 103, 77,
		24, 35, 55, 64, 81, 104, 113, 92,
		49, 64, 78, 87, 103, 121, 120, 101,
		72, 92, 95, 98, 112, 100, 103, 99,
	}
	if !isStandardTable(table[:], 0) {
		t.Error("standard luminance table should be recognized")
	}
}

func TestIsStandardTable_Chrominance(t *testing.T) {
	table := [64]byte{
		17, 18, 24, 47, 99, 99, 99, 99,
		18, 21, 26, 66, 99, 99, 99, 99,
		24, 26, 56, 99, 99, 99, 99, 99,
		47, 66, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
		99, 99, 99, 99, 99, 99, 99, 99,
	}
	if !isStandardTable(table[:], 0) {
		t.Error("standard chrominance table should be recognized")
	}
}

func TestIsStandardTable_NonStandard(t *testing.T) {
	table := make([]byte, 64)
	for i := range table {
		table[i] = 1
	}
	if isStandardTable(table[:], 0) {
		t.Error("constant-1 table should not be recognized as standard")
	}
}

func TestIsStandardTable_Short(t *testing.T) {
	if isStandardTable([]byte{1, 2, 3}, 0) {
		t.Error("short table should not be recognized as standard")
	}
}

func TestIsStandardTable_Precision16(t *testing.T) {
	// 16-bit precision: the function doesn't make detailed comparison
	// but still needs enough matched entries. Since all entries skip the
	// match logic (continue bypasses both lumMatch and chrMatch), neither
	// counter reaches the threshold → returns false.
	table := make([]byte, 64)
	result := isStandardTable(table, 1)
	// The current implementation returns false for 16-bit precision
	// because the continue skips all comparison logic
	if result {
		t.Error("16-bit precision table with zero entries should not match (all comparisons skipped)")
	}
}

// ── Helpers ────────────────────────────────────────────────────────────────

// simple RNG for deterministic test data
type rng struct {
	state uint64
}

func newRNG(seed uint64) *rng {
	return &rng{state: seed}
}

func (r *rng) next() uint64 {
	r.state ^= r.state >> 12
	r.state ^= r.state << 25
	r.state ^= r.state >> 27
	return r.state * 2685821657736338717
}

// ── Compile-time unused import suppression ──────────────────────────────────
var _ = math.NaN
