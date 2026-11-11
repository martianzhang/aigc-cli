package ocr

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

// NewDefaultEngine builds the standard offline OCR engine from modelsDir/ocr.
// modelsDir is the shared models root (the directory containing libonnxruntime
// and the ocr subdirectory).
func NewDefaultEngine(modelsDir, lang string) (*Engine, error) {
	ocrModelsDir := filepath.Join(modelsDir, "ocr")

	libPath, err := onnxrt.LibPath(modelsDir)
	if err != nil {
		libPath, err = onnxrt.EnsureInstalled(modelsDir, false)
		if err != nil {
			return nil, fmt.Errorf("ONNX Runtime not available: %w\n\nRun 'aigc-cli ocr init' first", err)
		}
	}

	detPath := filepath.Join(ocrModelsDir, "ch_PP-OCRv4_det_infer.onnx")
	recPath := filepath.Join(ocrModelsDir, "ch_PP-OCRv4_rec_infer.onnx")
	dictPath := filepath.Join(ocrModelsDir, "dict_zh.txt")

	if _, err := os.Stat(detPath); err != nil {
		return nil, fmt.Errorf("detection model not found: %w\n\nRun 'aigc-cli ocr init' first", err)
	}
	if _, err := os.Stat(recPath); err != nil {
		return nil, fmt.Errorf("recognition model not found: %w\n\nRun 'aigc-cli ocr init' first", err)
	}

	clsPath := filepath.Join(ocrModelsDir, "ch_ppocr_mobile_v2.0_cls_infer.onnx")
	enModelPath := filepath.Join(ocrModelsDir, "rec_en_PP-OCRv3_infer.onnx")
	enDictPath := filepath.Join(ocrModelsDir, "dict_en.txt")

	switch lang {
	case "zh":
		enModelPath, enDictPath = "", ""
	case "en":
	case "auto":
	default:
		return nil, fmt.Errorf("unsupported language %q, use: auto, zh, en", lang)
	}
	if enModelPath != "" {
		if _, err := os.Stat(enModelPath); err != nil {
			enModelPath = ""
		}
	}
	if enDictPath != "" {
		if _, err := os.Stat(enDictPath); err != nil {
			enDictPath = ""
		}
	}

	return NewEngine(libPath, detPath, recPath, clsPath, dictPath, 6625,
		"softmax_11.tmp_0", enModelPath, enDictPath, lang)
}
