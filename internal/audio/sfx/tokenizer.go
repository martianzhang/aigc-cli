package sfx

import (
	"fmt"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
)

// Tokenizer wraps the T5 fast tokenizer used to condition the audio model.
type Tokenizer struct {
	tk *tokenizer.Tokenizer
}

// NewTokenizer loads a HuggingFace tokenizer.json file.
func NewTokenizer(path string) (*Tokenizer, error) {
	tk, err := pretrained.FromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}
	return &Tokenizer{tk: tk}, nil
}

// Encode returns exactly TextTokens ids and an attention mask. The tokenizer
// already pads to its configured length; fitTokens is defensive about both
// shorter and longer sequences.
func (t *Tokenizer) Encode(text string) ([]int64, []int64, error) {
	enc, err := t.tk.EncodeSingle(text, true)
	if err != nil {
		return nil, nil, fmt.Errorf("encode prompt: %w", err)
	}
	return fitTokens(enc.GetIds(), enc.GetAttentionMask(), TextTokens)
}

// fitTokens pads or truncates a token sequence to n. Truncated sequences keep a
// trailing EOS token so the T5 encoder still sees a well-formed sequence.
func fitTokens(ids, mask []int, n int) ([]int64, []int64, error) {
	if n <= 0 {
		return nil, nil, fmt.Errorf("token length must be positive")
	}
	if len(mask) == 0 {
		mask = make([]int, len(ids))
		for i := range mask {
			mask[i] = 1
		}
	}

	outIDs := make([]int64, n)
	outMask := make([]int64, n)
	limit := len(ids)
	if limit > n {
		limit = n
	}
	for i := 0; i < limit; i++ {
		outIDs[i] = int64(ids[i])
		outMask[i] = int64(mask[i])
	}
	if len(ids) > n {
		// Keep EOS (id 1 for T5) at the end of a truncated prompt.
		outIDs[n-1] = 1
		outMask[n-1] = 1
	}
	return outIDs, outMask, nil
}
