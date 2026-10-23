package sfx

import "testing"

func TestFitTokensPads(t *testing.T) {
	ids, mask, err := fitTokens([]int{5, 6, 7, 1}, nil, 8)
	if err != nil {
		t.Fatalf("fitTokens() error = %v", err)
	}
	if len(ids) != 8 || len(mask) != 8 {
		t.Fatalf("lengths = %d/%d, want 8/8", len(ids), len(mask))
	}
	wantIDs := []int64{5, 6, 7, 1, 0, 0, 0, 0}
	wantMask := []int64{1, 1, 1, 1, 0, 0, 0, 0}
	for i := range wantIDs {
		if ids[i] != wantIDs[i] {
			t.Errorf("ids[%d] = %d, want %d", i, ids[i], wantIDs[i])
		}
		if mask[i] != wantMask[i] {
			t.Errorf("mask[%d] = %d, want %d", i, mask[i], wantMask[i])
		}
	}
}

func TestFitTokensUsesProvidedMask(t *testing.T) {
	_, mask, err := fitTokens([]int{5, 6, 7}, []int{1, 1, 0}, 4)
	if err != nil {
		t.Fatalf("fitTokens() error = %v", err)
	}
	if mask[2] != 0 {
		t.Errorf("mask[2] = %d, want 0", mask[2])
	}
	if mask[3] != 0 {
		t.Errorf("mask[3] = %d, want 0", mask[3])
	}
}

func TestFitTokensTruncatesAndKeepsEOS(t *testing.T) {
	ids, mask, err := fitTokens([]int{5, 6, 7, 8, 9}, nil, 4)
	if err != nil {
		t.Fatalf("fitTokens() error = %v", err)
	}
	if ids[3] != 1 {
		t.Errorf("last id = %d, want EOS (1)", ids[3])
	}
	if mask[3] != 1 {
		t.Errorf("last mask = %d, want 1", mask[3])
	}
}

func TestFitTokensRejectsZeroLength(t *testing.T) {
	if _, _, err := fitTokens([]int{1}, nil, 0); err == nil {
		t.Fatal("fitTokens() error = nil, want error")
	}
}

func TestRequestValidate(t *testing.T) {
	if err := (Request{Prompt: "door", Seconds: 1}).Validate(); err != nil {
		t.Errorf("valid request rejected: %v", err)
	}
	for _, req := range []Request{
		{Seconds: 1},
		{Prompt: "door", Seconds: 0},
		{Prompt: "door", Seconds: MaxSeconds + 0.1},
	} {
		if err := req.Validate(); err == nil {
			t.Errorf("Request%+v.Validate() = nil, want error", req)
		}
	}
}

func TestAudioDuration(t *testing.T) {
	a := &Audio{Samples: make([]float32, 2*44100), SampleRate: 44100, Channels: 2}
	if got := a.Duration(); got != 1 {
		t.Errorf("Duration() = %v, want 1", got)
	}
	var nilAudio *Audio
	if got := nilAudio.Duration(); got != 0 {
		t.Errorf("nil Duration() = %v, want 0", got)
	}
}
