//go:build !cgo

package audio

import "errors"

var errNoCGO = errors.New("local TTS requires CGO (not available in this build)")

func NewTTSEngine(_, _ string) (*TTSEngine, error)               { return nil, errNoCGO }
func (e *TTSEngine) NumSpeakers() int                            { return 0 }
func (e *TTSEngine) Speak(_ string, _ int) ([]int16, int, error) { return nil, 0, errNoCGO }
func (e *TTSEngine) Close()                                      {}
