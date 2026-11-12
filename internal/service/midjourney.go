package service

import "encoding/json"

// MJInjector is implemented by every Midjourney request body (via
// SetInjectJSON) so --inject-json can be attached without a per-type switch.
type MJInjector interface {
	SetInjectJSON(json.RawMessage)
}

// ApplyMJInject attaches injectJSON to req when the value is non-empty.
func ApplyMJInject(req any, injectJSON string) error {
	inject, err := ReadInjectJSON(injectJSON)
	if err != nil {
		return err
	}
	if len(inject) == 0 {
		return nil
	}
	if s, ok := req.(MJInjector); ok {
		s.SetInjectJSON(inject)
	}
	return nil
}
