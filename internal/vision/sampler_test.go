package vision

import (
	"testing"
)

// ── getKthLargest ──

func TestGetKthLargest_basic(t *testing.T) {
	got := getKthLargest([]float64{1, 3, 5, 7, 9}, 3)
	want := 5.0
	if got != want {
		t.Errorf("getKthLargest([1,3,5,7,9], 3) = %v, want %v", got, want)
	}
}

func TestGetKthLargest_k1(t *testing.T) {
	got := getKthLargest([]float64{4, 2, 8, 6, 1}, 1)
	want := 8.0
	if got != want {
		t.Errorf("getKthLargest k=1 (max) = %v, want %v", got, want)
	}
}

func TestGetKthLargest_kN(t *testing.T) {
	got := getKthLargest([]float64{1, 2, 3}, 3)
	want := 1.0
	if got != want {
		t.Errorf("getKthLargest k=n (min) = %v, want %v", got, want)
	}
}

func TestGetKthLargest_empty(t *testing.T) {
	got := getKthLargest([]float64{}, 1)
	if got != 0 {
		t.Errorf("getKthLargest([]) = %v, want 0", got)
	}
}

func TestGetKthLargest_kOutOfBounds(t *testing.T) {
	got := getKthLargest([]float64{1, 2, 3}, 0)
	if got != 0 {
		t.Errorf("getKthLargest k=0 = %v, want 0", got)
	}
	got = getKthLargest([]float64{1, 2, 3}, 10)
	if got != 0 {
		t.Errorf("getKthLargest k>len = %v, want 0", got)
	}
}

func TestGetKthLargest_singleElement(t *testing.T) {
	got := getKthLargest([]float64{42.5}, 1)
	if got != 42.5 {
		t.Errorf("single element = %v, want 42.5", got)
	}
}

func TestGetKthLargest_negativeValues(t *testing.T) {
	got := getKthLargest([]float64{-5, -1, -10, -3}, 2)
	want := -3.0
	if got != want {
		t.Errorf("negative values k=2 = %v, want %v", got, want)
	}
}

// ── Sampler ──

func TestGreedy_basic(t *testing.T) {
	s := &Sampler{}
	logits := []float32{0.1, 0.8, 0.2, 0.5}
	got := s.greedy(logits)
	want := int64(1)
	if got != want {
		t.Errorf("greedy([0.1, 0.8, 0.2, 0.5]) = %d, want %d", got, want)
	}
}

func TestGreedy_allSame(t *testing.T) {
	s := &Sampler{}
	logits := []float32{1.0, 1.0, 1.0}
	got := s.greedy(logits)
	if got != 0 {
		t.Errorf("greedy all-same = %d, want 0 (first)", got)
	}
}

func TestGreedy_single(t *testing.T) {
	s := &Sampler{}
	logits := []float32{42.0}
	got := s.greedy(logits)
	if got != 0 {
		t.Errorf("greedy single = %d, want 0", got)
	}
}

func TestGreedy_negativeLogits(t *testing.T) {
	s := &Sampler{}
	logits := []float32{-5.0, -1.0, -3.0}
	got := s.greedy(logits)
	want := int64(1)
	if got != want {
		t.Errorf("greedy negative logits = %d, want %d", got, want)
	}
}

func TestApplyRepetitionPenalty_positive(t *testing.T) {
	s := &Sampler{RepetitionPenalty: 2.0}
	logits := []float32{1.0, 2.0, 3.0}
	s.applyRepetitionPenalty(logits, []int64{1}) // token 1 appeared
	if logits[1] != 1.0 {
		t.Errorf("repetition penalty on positive: got %v, want 1.0", logits[1])
	}
	// Others unchanged
	if logits[0] != 1.0 || logits[2] != 3.0 {
		t.Errorf("other logits changed: %v", logits)
	}
}

func TestApplyRepetitionPenalty_negative(t *testing.T) {
	s := &Sampler{RepetitionPenalty: 2.0}
	logits := []float32{-1.0, -2.0, -3.0}
	s.applyRepetitionPenalty(logits, []int64{0}) // token 0 appeared, logit is -1
	// negative * 2.0 = -2.0
	if logits[0] != -2.0 {
		t.Errorf("repetition penalty on negative: got %v, want -2.0", logits[0])
	}
}

func TestApplyRepetitionPenalty_multipleTokens(t *testing.T) {
	s := &Sampler{RepetitionPenalty: 2.0}
	logits := []float32{1.0, 2.0, 3.0, 4.0}
	s.applyRepetitionPenalty(logits, []int64{0, 2}) // tokens 0 and 2
	if logits[0] != 0.5 {
		t.Errorf("token 0: got %v, want 0.5", logits[0])
	}
	if logits[2] != 1.5 {
		t.Errorf("token 2: got %v, want 1.5", logits[2])
	}
	if logits[1] != 2.0 || logits[3] != 4.0 {
		t.Errorf("other logits changed: %v", logits)
	}
}

func TestSample_greedy_noRepetitionPenalty(t *testing.T) {
	s := &Sampler{Temperature: 0.0, RepetitionPenalty: 0}
	logits := []float32{0.1, 0.9, 0.2}
	got := s.Sample(logits, nil)
	if got != 1 {
		t.Errorf("Sample greedy = %d, want 1", got)
	}
}

func TestSample_greedy_withRepetitionPenalty(t *testing.T) {
	s := &Sampler{Temperature: 0.0, RepetitionPenalty: 2.0}
	// After rep penalty: [0.5, 0.9, 0.2] → still picks index 1
	logits := []float32{1.0, 0.9, 0.2}
	got := s.Sample(logits, []int64{0})
	if got != 1 {
		t.Errorf("Sample greedy+repPenalty = %d, want 1", got)
	}
}

func TestSample_emptyLogits(t *testing.T) {
	s := &Sampler{Temperature: 0.0}
	got := s.Sample(nil, nil)
	if got != 0 {
		t.Errorf("Sample empty logits = %d, want 0", got)
	}
	got = s.Sample([]float32{}, []int64{})
	if got != 0 {
		t.Errorf("Sample empty slice = %d, want 0", got)
	}
}

func TestSample_topK_basic(t *testing.T) {
	s := &Sampler{Temperature: 1.0, TopK: 3}
	logits := []float32{0.1, 0.8, 0.2, 0.5, 0.3}
	got := s.topKLogits(logits)
	// With temperature=1.0 and softmax, index 1 has the highest score
	if got != 1 {
		t.Errorf("topK logits = %d, want 1", got)
	}
}

func TestSample_temperatureScaled(t *testing.T) {
	s := &Sampler{Temperature: 0.001, TopK: 0}
	// Very small temperature → treated as ~0.001 → still goes to topK path
	logits := []float32{0.1, 0.9, 0.0}
	got := s.Sample(logits, nil)
	// Temperature 0.001 < 0.001 is false (0.001 == 0.001), so it goes to topK
	// With temperature=0.001, scaling amplifies: [100, 900, 0]
	// Softmax: index 1 dominates → picks 1
	if got != 1 {
		t.Errorf("tiny temperature = %d, want 1", got)
	}
}

func TestSample_topKWithRepetition(t *testing.T) {
	s := &Sampler{Temperature: 1.0, TopK: 3, RepetitionPenalty: 2.0}
	// Token 1 already generated, penalty should reduce it
	logits := []float32{1.0, 2.0, 3.0, 0.5}
	got := s.Sample(logits, []int64{1})
	// After rep penalty: [1.0, 1.0, 3.0, 0.5]. Top-k picks highest → index 2
	if got == 1 {
		t.Errorf("repetition shouldn't pick the penalized token, got %d", got)
	}
}

// ── NewSampler / DefaultSampler ──

func TestNewSampler_defaults(t *testing.T) {
	s := NewSampler(1.0, 10, 1.2)
	if s.Temperature != 1.0 {
		t.Errorf("Temp = %v, want 1.0", s.Temperature)
	}
	if s.TopK != 10 {
		t.Errorf("TopK = %d, want 10", s.TopK)
	}
	if s.RepetitionPenalty != 1.2 {
		t.Errorf("RepPen = %v, want 1.2", s.RepetitionPenalty)
	}
}

func TestDefaultSampler(t *testing.T) {
	s := DefaultSampler()
	if s.Temperature != 0.0 {
		t.Errorf("Temp = %v, want 0.0", s.Temperature)
	}
	if s.TopK != 0 {
		t.Errorf("TopK = %d, want 0", s.TopK)
	}
	if s.RepetitionPenalty != 1.2 {
		t.Errorf("RepPen = %v, want 1.2", s.RepetitionPenalty)
	}
}

// ── ResolveModelVariant ──

func TestResolveModelVariant_found(t *testing.T) {
	v, err := ResolveModelVariant("base-int8")
	if err != nil {
		t.Fatalf("ResolveModelVariant = %v", err)
	}
	if v.ID != "base-int8" {
		t.Errorf("ID = %q, want %q", v.ID, "base-int8")
	}
	if v.Description != "Florence-2 Base INT8 (quantized)" {
		t.Errorf("Desc = %q", v.Description)
	}
}

func TestResolveModelVariant_unknown(t *testing.T) {
	_, err := ResolveModelVariant("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown variant")
	}
	if err.Error() != `unknown model variant "nonexistent" (available: base-int8)` {
		t.Errorf("error = %q, want %q", err.Error(), `unknown model variant "nonexistent" (available: base-int8)`)
	}
}

func TestResolveModelVariant_empty(t *testing.T) {
	v, err := ResolveModelVariant("")
	if err == nil {
		t.Fatal("expected error for empty variant")
	}
	if v.ID != "" || v.Description != "" {
		t.Errorf("empty variant returned non-zero: %+v", v)
	}
}

// ── ExpandPrompt ──

func TestExpandPrompt_caption(t *testing.T) {
	got := ExpandPrompt("<CAPTION>")
	want := "What does the image describe?"
	if got != want {
		t.Errorf("ExpandPrompt(CAPTION) = %q, want %q", got, want)
	}
}

func TestExpandPrompt_detailedCaption(t *testing.T) {
	got := ExpandPrompt("<DETAILED_CAPTION>")
	want := "Describe in detail what is shown in the image."
	if got != want {
		t.Errorf("ExpandPrompt(DETAILED) = %q, want %q", got, want)
	}
}

func TestExpandPrompt_moreDetailedCaption(t *testing.T) {
	got := ExpandPrompt("<MORE_DETAILED_CAPTION>")
	want := "Describe with a paragraph what is shown in the image."
	if got != want {
		t.Errorf("ExpandPrompt(MORE_DETAILED) = %q, want %q", got, want)
	}
}

func TestExpandPrompt_od(t *testing.T) {
	got := ExpandPrompt("<OD>")
	want := "Locate the objects with category name in the image."
	if got != want {
		t.Errorf("ExpandPrompt(OD) = %q, want %q", got, want)
	}
}

func TestExpandPrompt_unknown(t *testing.T) {
	got := ExpandPrompt("<VQA>")
	want := "<VQA>"
	if got != want {
		t.Errorf("ExpandPrompt(unknown) = %q, want %q", got, want)
	}
}

func TestExpandPrompt_empty(t *testing.T) {
	got := ExpandPrompt("")
	if got != "" {
		t.Errorf("ExpandPrompt(\"\") = %q, want %q", got, "")
	}
}

// ── constants ──

func TestModelConstants(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{"InputSize", InputSize, 768},
		{"Channels", Channels, 3},
		{"HiddenDim", HiddenDim, 768},
		{"NumDecoderLayers", NumDecoderLayers, 6},
		{"NumAttentionHeads", NumAttentionHeads, 12},
		{"HeadDim", HeadDim, 64},
		{"ImageSeqLength", ImageSeqLength, 577},
		{"MaxTokens", MaxTokens, 512},
		{"VocabSize", VocabSize, 51289},
		{"DecoderStartTokenID", DecoderStartTokenID, 2},
		{"EOSTokenID", EOSTokenID, 2},
		{"PadTokenID", PadTokenID, 1},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestModelVariantsList(t *testing.T) {
	if len(AvailableModelVariants) == 0 {
		t.Fatal("AvailableModelVariants is empty")
	}
	v := AvailableModelVariants[0]
	if v.ID != "base-int8" {
		t.Errorf("First variant ID = %q, want %q", v.ID, "base-int8")
	}
	if v.Size != "~270 MB" {
		t.Errorf("Size = %q, want %q", v.Size, "~270 MB")
	}
}

// ── PreprocessImageFromImage constants ──

func TestImageNetNormalizationParams(t *testing.T) {
	expectedMean := [3]float32{0.485, 0.456, 0.406}
	expectedStd := [3]float32{0.229, 0.224, 0.225}
	if MeanRGB != expectedMean {
		t.Errorf("MeanRGB = %v, want %v", MeanRGB, expectedMean)
	}
	if StdRGB != expectedStd {
		t.Errorf("StdRGB = %v, want %v", StdRGB, expectedStd)
	}
}

// ── TopKLogits: test softmax behavior ──

func TestTopKLogits_temperatureAmplifies(t *testing.T) {
	s := &Sampler{Temperature: 0.5, TopK: 0}
	// Temperature 0.5 → doubles the logits
	// [0.1, 0.9] → scaled to [0.2, 1.8]
	// Softmax heavily favors index 1
	logits := []float32{-10, 10}
	got := s.topKLogits(logits)
	if got != 1 {
		t.Errorf("amplified logits = %d, want 1", got)
	}
}

func TestTopKLogits_lowTemperatureStaysSafe(t *testing.T) {
	s := &Sampler{Temperature: 0.0001, TopK: 0}
	// Very tiny temperature → clamped to 0.001
	logits := []float32{0.5, 0.0, -0.5}
	got := s.topKLogits(logits) // should pick index 0 (highest after scaling)
	if got != 0 {
		t.Errorf("clamp temp logits = %d, want 0", got)
	}
}

func TestTopKLogits_flatLogits(t *testing.T) {
	s := &Sampler{Temperature: 1.0, TopK: 0}
	// All same → softmax gives uniform distribution → picks first
	logits := []float32{1.0, 1.0, 1.0, 1.0}
	got := s.topKLogits(logits)
	if got != 0 {
		t.Errorf("flat logits = %d, want 0 (first tie)", got)
	}
}

func TestTopKLogits_withTopKFiltration(t *testing.T) {
	s := &Sampler{Temperature: 1.0, TopK: 2}
	logits := []float32{0.1, 0.5, 0.9, 0.0}
	got := s.topKLogits(logits)
	// Top-2 picks from indices with highest softmax: 1 and 2 → picks 2
	if got != 2 {
		t.Errorf("topK=2 = %d, want 2", got)
	}
}

func TestTopKLogits_filteredSumZero(t *testing.T) {
	s := &Sampler{Temperature: 1.0, TopK: 0}
	// When all filtered probs are zero (all negative → softmax to 0), falls back to greedy
	logits := []float32{-10, -20, -30}
	got := s.topKLogits(logits)
	// Greedy picks highest → index 0
	if got != 0 {
		t.Errorf("fallback greedy = %d, want 0", got)
	}
}

// ── getKthLargest: edge cases ──

func TestGetKthLargest_duplicates(t *testing.T) {
	got := getKthLargest([]float64{5, 5, 5, 5}, 2)
	want := 5.0
	if got != want {
		t.Errorf("duplicates k=2 = %v, want %v", got, want)
	}
}

func TestGetKthLargest_sorted(t *testing.T) {
	got := getKthLargest([]float64{1, 2, 3, 4, 5}, 3)
	want := 3.0
	if got != want {
		t.Errorf("sorted k=3 = %v, want %v", got, want)
	}
}

func TestGetKthLargest_reverseSorted(t *testing.T) {
	got := getKthLargest([]float64{5, 4, 3, 2, 1}, 3)
	want := 3.0
	if got != want {
		t.Errorf("reverse sorted k=3 = %v, want %v", got, want)
	}
}

// ── Sampler edge: rep penalty ignores out-of-vocab token ──

func TestApplyRepetitionPenalty_outOfBounds(t *testing.T) {
	s := &Sampler{RepetitionPenalty: 2.0}
	logits := []float32{1.0, 2.0}
	// id = 100 is out of bounds → skip
	s.applyRepetitionPenalty(logits, []int64{100})
	if logits[0] != 1.0 || logits[1] != 2.0 {
		t.Errorf("out-of-bounds ID should not change logits: %v", logits)
	}
}

func TestApplyRepetitionPenalty_negativeId(t *testing.T) {
	s := &Sampler{RepetitionPenalty: 2.0}
	logits := []float32{1.0, 2.0}
	// id = -1 → out of bounds (negative) → skip
	s.applyRepetitionPenalty(logits, []int64{-1})
	if logits[0] != 1.0 || logits[1] != 2.0 {
		t.Errorf("negative ID should not change logits: %v", logits)
	}
}

// ── topK: k clipped to len ──

func TestTopKLogits_kExceedsVocab(t *testing.T) {
	s := &Sampler{Temperature: 1.0, TopK: 100}
	logits := []float32{0.1, 0.8, 0.2}
	got := s.topKLogits(logits)
	// k=100 > len=3 → k clipped to 3 → all tokens considered
	if got != 1 {
		t.Errorf("k>len = %d, want 1", got)
	}
}

// ── getKthLargest: large k ──

func TestGetKthLargest_largeK(t *testing.T) {
	got := getKthLargest([]float64{1, 2, 3}, 100)
	if got != 0 {
		t.Errorf("k>>len = %v, want 0", got)
	}
}
