package audio

import "unsafe"

// ASREngine wraps sherpa-onnx's OfflineRecognizer for local speech recognition.
type ASREngine struct {
	impl     unsafe.Pointer
	modelDir string
}
