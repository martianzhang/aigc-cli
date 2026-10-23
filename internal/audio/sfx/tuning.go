package sfx

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Tuning holds engine-native knobs passed through --inject-json for local
// generation. Unknown keys are rejected so a typo never silently changes
// nothing.
type Tuning struct {
	Steps     int     `json:"steps"`
	SigmaMax  float64 `json:"sigma_max"`
	Seed      *int64  `json:"seed"`
	Normalize *bool   `json:"normalize"`
}

// ParseTuning decodes local engine knobs from an --inject-json object.
func ParseTuning(raw json.RawMessage) (Tuning, error) {
	var t Tuning
	if len(raw) == 0 {
		return t, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return Tuning{}, fmt.Errorf("local sfx supports only steps, sigma_max, seed and normalize: %w", err)
	}
	if t.Steps < 0 || t.Steps > 50 {
		return Tuning{}, fmt.Errorf("local sfx steps must be 1-50, got %d", t.Steps)
	}
	if t.SigmaMax < 0 || t.SigmaMax > 1 {
		return Tuning{}, fmt.Errorf("local sfx sigma_max must be 0.01-1, got %v", t.SigmaMax)
	}
	if t.SigmaMax > 0 && t.SigmaMax < 0.01 {
		return Tuning{}, fmt.Errorf("local sfx sigma_max must be 0.01-1, got %v", t.SigmaMax)
	}
	return t, nil
}
