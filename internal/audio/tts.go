package audio

import "unsafe"

// TTSEngine wraps sherpa-onnx's OfflineTts for local TTS inference.
// Implementation in tts_sherpa.go (purego FFI, no CGO needed).
type TTSEngine struct {
	sr       int
	modelDir string
	impl     unsafe.Pointer // C TTS engine handle (set by FFI implementation)
}
