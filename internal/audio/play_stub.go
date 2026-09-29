//go:build !cgo && !darwin && !windows

// Fallback for platforms without an in-process audio backend: a Linux/BSD build
// made with CGO_ENABLED=0 has no ALSA driver, so oto cannot compile. Rather
// than fail the build, playback returns ErrPlaybackUnavailable and callers
// degrade gracefully (preview prints a warning; commands that need playback
// can fall back to an external player).

package audio

import "fmt"

// PlayAudioFile reports that in-process playback is unavailable. The file is
// still decoded first so malformed inputs fail with the usual decode error.
func PlayAudioFile(path string) error {
	if _, err := DecodeAudioFile(path); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return ErrPlaybackUnavailable
}

// PlayAudioData reports that in-process playback is unavailable.
func PlayAudioData(_ *AudioData) error {
	return ErrPlaybackUnavailable
}
