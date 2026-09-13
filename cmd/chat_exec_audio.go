package cmd

import (
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/ocr"
	"github.com/martianzhang/aigc-cli/internal/pdf"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// executeGenerateSpeech runs TTS and returns a text summary for the LLM.
func executeGenerateSpeech(argsJSON string) string {
	var params struct {
		Input  string `json:"input"`
		Model  string `json:"model"`
		Voice  string `json:"voice"`
		Format string `json:"format"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if params.Input == "" {
		return "Error: input is required"
	}
	if params.Voice == "" {
		return "Error: voice is required"
	}
	model := params.Model
	if model == "" {
		if cfg := audioDefaults(); cfg != nil && cfg.SpeakModel != "" {
			model = cfg.SpeakModel
		} else {
			model = "gpt-4o-mini-tts"
		}
	}
	format := params.Format
	if format == "" {
		format = "mp3"
	}

	req := &types.AudioSpeechRequest{
		Model:          model,
		Input:          params.Input,
		Voice:          params.Voice,
		ResponseFormat: format,
	}

	c := newCmdClient("chat")
	applyTimeout(c, "audio", client.AudioTimeout)

	audioData, _, err := c.AudioSpeech(req)
	if err != nil {
		return fmt.Sprintf("Error: TTS failed: %v", err)
	}

	filename, err := saveAudioFile(audioData, format)
	if err != nil {
		return fmt.Sprintf("Error: failed to save audio: %v", err)
	}

	return fmt.Sprintf("Speech generated and saved to: %s\nFormat: %s\nSize: %d bytes\nModel: %s\nVoice: %s",
		filename, format, len(audioData), model, params.Voice)
}

// executeTranscribeAudio runs STT and returns a text summary for the LLM.
func executeTranscribeAudio(argsJSON string) string {
	var params struct {
		FilePath string `json:"file_path"`
		Model    string `json:"model"`
		Language string `json:"language"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if params.FilePath == "" {
		return "Error: file_path is required"
	}
	model := params.Model
	if model == "" {
		if cfg := audioDefaults(); cfg != nil && cfg.TranscribeModel != "" {
			model = cfg.TranscribeModel
		} else {
			model = "whisper-1"
		}
	}

	c := newCmdClient("chat")
	applyTimeout(c, "audio", client.AudioTimeout)

	resp, err := c.AudioTranscribeMultipart(model, params.FilePath, params.Language)
	if err != nil {
		return fmt.Sprintf("Error: STT failed: %v", err)
	}

	result := fmt.Sprintf("Transcription result (model: %s):\n%s", model, resp.Text)
	if resp.Usage != nil && resp.Usage.Cost > 0 {
		result += fmt.Sprintf("\n(Cost: $%.5f)", resp.Usage.Cost)
	}
	return result
}

// executeRecognizeText runs OCR on a local image or PDF and returns recognized text.
func executeRecognizeText(argsJSON string) string {
	var args struct {
		FilePath string `json:"file_path"`
		Lang     string `json:"lang"`
		Format   string `json:"format"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.FilePath == "" {
		return "Error: file_path is required"
	}

	path := args.FilePath
	if !filepath.IsAbs(path) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return fmt.Sprintf("Error: invalid path: %v", err)
		}
		path = abs
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Sprintf("Error: file not found: %s", path)
	}

	if args.Lang == "" {
		args.Lang = "auto"
	}
	if args.Format == "" {
		args.Format = "text"
	}

	modelsDir := defaultModelsDir()
	engine, err := newOCREngineWithLang(modelsDir, args.Lang)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer engine.Close()

	var result *ocr.OCRResult

	if strings.EqualFold(filepath.Ext(path), ".pdf") {
		// Handle PDF via text extraction → fallback to OCR
		pages, extractErr := pdf.ExtractText(path)
		if extractErr != nil {
			return fmt.Sprintf("Error: cannot read PDF: %v", extractErr)
		}

		if !pdf.IsScanned(pages) {
			var textLines []string
			ocrPages := make([]ocr.OCRPage, 0, len(pages))
			for _, p := range pages {
				line := strings.TrimSpace(p.Text)
				if line != "" {
					textLines = append(textLines, line)
					ocrPages = append(ocrPages, ocr.OCRPage{
						Page: p.Page - 1,
						Lines: []ocr.OCRLine{{
							Text:       line,
							Confidence: 1.0,
						}},
					})
				}
			}
			result = &ocr.OCRResult{
				Pages: ocrPages,
				Text:  strings.Join(textLines, "\n"),
			}
		} else {
			tmpDir, tmpErr := os.MkdirTemp("", "aigc-cli-pdf-*")
			if tmpErr != nil {
				return fmt.Sprintf("Error: create temp dir: %v", tmpErr)
			}
			defer os.RemoveAll(tmpDir)

			pngs, renderErr := pdf.RenderToImages(path, tmpDir, 300)
			if renderErr != nil {
				return fmt.Sprintf("Error: render PDF: %v", renderErr)
			}

			allPages := make([]ocr.OCRPage, 0, len(pngs))
			allText := make([]string, 0, len(pngs))
			for pageIdx, pngPath := range pngs {
				f, openErr := os.Open(pngPath)
				if openErr != nil {
					return fmt.Sprintf("Error: open rendered page %d: %v", pageIdx+1, openErr)
				}
				img, _, decodeErr := image.Decode(f)
				f.Close()
				if decodeErr != nil {
					return fmt.Sprintf("Error: decode rendered page %d: %v", pageIdx+1, decodeErr)
				}
				pageResult, scanErr := engine.Scan(img)
				if scanErr != nil {
					return fmt.Sprintf("Error: OCR page %d: %v", pageIdx+1, scanErr)
				}
				for i := range pageResult.Pages {
					pageResult.Pages[i].Page = pageIdx
				}
				allPages = append(allPages, pageResult.Pages...)
				allText = append(allText, pageResult.Text)
			}
			result = &ocr.OCRResult{
				Pages: allPages,
				Text:  strings.Join(allText, "\n"),
			}
		}
	} else {
		f, openErr := os.Open(path)
		if openErr != nil {
			return fmt.Sprintf("Error: cannot open file: %v", openErr)
		}
		img, _, decodeErr := image.Decode(f)
		f.Close()
		if decodeErr != nil {
			return fmt.Sprintf("Error: cannot decode image: %v", decodeErr)
		}
		result, err = engine.Scan(img)
		if err != nil {
			return fmt.Sprintf("Error: OCR failed: %v", err)
		}
	}

	if args.Format == "json" {
		data, _ := json.MarshalIndent(result, "", "  ")
		return string(data)
	}

	if result.Text == "" {
		return "(no text detected)"
	}
	return result.Text
}
