package audio

import "errors"

// ErrPlaybackUnavailable is returned when there is no in-process audio backend:
// a Linux/BSD build made without CGO + ALSA (see play_stub.go), or an audio
// device that oto could not open. Callers may fall back to an external player.
var ErrPlaybackUnavailable = errors.New("audio playback unavailable (Linux/BSD builds need CGO + libasound2-dev; no audio device?)")
