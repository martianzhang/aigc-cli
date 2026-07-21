package audio

// ASREngine wraps sherpa-onnx's OfflineRecognizer for local speech recognition.
type ASREngine struct {
	modelDir string
}
