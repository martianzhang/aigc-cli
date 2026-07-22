package ocr

// ModelInfo describes a downloadable OCR model.
type ModelInfo struct {
	ID          string      // "rapidocr"
	Name        string      // human-readable name
	Description string      // short description
	Files       []ModelFile // model files to download
}

// ModelFile describes a single model file.
type ModelFile struct {
	Type    string // "det", "rec", "cls", "dict"
	URL     string // download URL
	SizeMB  int64  // approximate size in MB
	OutName string // output filename
}

const modelsBaseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"

// Models returns the available OCR model packs.
func Models() []ModelInfo {
	return []ModelInfo{
		{
			ID:          "rapidocr",
			Name:        "RapidOCR 中文",
			Description: "Chinese/English text detection + recognition (PP-OCRv4 mobile)",
			Files: []ModelFile{
				{Type: "det", URL: modelsBaseURL + "/ch_PP-OCRv4_det_infer.onnx", SizeMB: 5, OutName: "ch_PP-OCRv4_det_infer.onnx"},
				{Type: "rec", URL: modelsBaseURL + "/ch_PP-OCRv4_rec_infer.onnx", SizeMB: 10, OutName: "ch_PP-OCRv4_rec_infer.onnx"},
				{Type: "rec_en", URL: modelsBaseURL + "/rec_en_PP-OCRv3_infer.onnx", SizeMB: 9, OutName: "rec_en_PP-OCRv3_infer.onnx"},
				{Type: "dict", URL: modelsBaseURL + "/dict_zh.txt", SizeMB: 0, OutName: "dict_zh.txt"},
				{Type: "dict_en", URL: modelsBaseURL + "/dict_en.txt", SizeMB: 0, OutName: "dict_en.txt"},
				{Type: "cls", URL: modelsBaseURL + "/ch_ppocr_mobile_v2.0_cls_infer.onnx", SizeMB: 1, OutName: "ch_ppocr_mobile_v2.0_cls_infer.onnx"},
			},
		},
	}
}

// FindModelByID returns the model info for the given ID.
func FindModelByID(id string) (ModelInfo, bool) {
	for _, m := range Models() {
		if m.ID == id {
			return m, true
		}
	}
	return ModelInfo{}, false
}
