package audio

// TTSEngine wraps sherpa-onnx's OfflineTts for local TTS inference.
// Real implementation: tts_sherpa.go (//go:build cgo)
// Stub:             tts_stub.go  (//go:build !cgo)
type TTSEngine struct {
	sr        int
	modelDir  string // set by sherpa implementation
}
