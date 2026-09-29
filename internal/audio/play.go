package audio

import "errors"

// ErrPlaybackUnavailable is returned when no audio backend could play the
// sound: no in-process device (oto on macOS/Windows) and no external player
// command (Linux/BSD). Callers may fall back to opening the system handler.
var ErrPlaybackUnavailable = errors.New("audio playback unavailable (no audio device or external player found)")
