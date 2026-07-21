//go:build !cgo

package audio

import "errors"

var errNoCGOasr = errors.New("local ASR requires CGO (not available in this build)")

func NewASREngine(_, _ string) (*ASREngine, error)       { return nil, errNoCGOasr }
func (e *ASREngine) Transcribe(_ string) (string, error) { return "", errNoCGOasr }
func (e *ASREngine) Close()                              {}
