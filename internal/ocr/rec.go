package ocr

import (
	"fmt"
	"image"
)

// Recognize recognizes text in a single image region using Chinese model.
func (e *Engine) Recognize(img image.Image, box [4][2]int) (*OCRLine, error) {
	// Preprocess
	pixels, regionWidth := recPreprocess(img, box)
	if pixels == nil || regionWidth <= 0 {
		return nil, fmt.Errorf("invalid text region")
	}

	// Copy to input tensor
	data := e.recIn.GetData()
	if len(data) != len(pixels) {
		return nil, fmt.Errorf("rec input tensor size mismatch: got %d want %d", len(data), len(pixels))
	}
	copy(data, pixels)

	// Run inference
	if err := e.rec.Run(); err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	// Read output
	outData := e.recOut.GetData()
	maxTimestep := regionWidth / 8
	if maxTimestep <= 0 {
		maxTimestep = 1
	}
	if maxTimestep > RecMaxWidth/8 {
		maxTimestep = RecMaxWidth / 8
	}

	// Greedy CTC decoding
	text := ctcGreedyDecode(outData, maxTimestep, e.recVocabSize, e.dict)

	// Confidence
	var totalConf float32
	for t := 0; t < maxTimestep; t++ {
		offset := t * e.recVocabSize
		maxVal := outData[offset]
		for c := 1; c < e.recVocabSize; c++ {
			if outData[offset+c] > maxVal {
				maxVal = outData[offset+c]
			}
		}
		totalConf += maxVal
	}
	confidence := totalConf / float32(maxTimestep)

	return &OCRLine{
		Text:       text,
		BBox:       box,
		Confidence: confidence,
	}, nil
}

// RecognizeEN recognizes English text using the dedicated English model.
func (e *Engine) RecognizeEN(img image.Image, box [4][2]int) (*OCRLine, error) {
	if e.enRec == nil {
		return e.Recognize(img, box)
	}

	pixels, regionWidth := recPreprocess(img, box)
	if pixels == nil || regionWidth <= 0 {
		return nil, fmt.Errorf("invalid text region")
	}

	data := e.enIn.GetData()
	if len(data) != len(pixels) {
		return nil, fmt.Errorf("en input tensor size mismatch: got %d want %d", len(data), len(pixels))
	}
	copy(data, pixels)

	if err := e.enRec.Run(); err != nil {
		return nil, fmt.Errorf("en inference failed: %w", err)
	}

	outData := e.enOut.GetData()
	maxTimestep := regionWidth / 8
	if maxTimestep <= 0 {
		maxTimestep = 1
	}
	if maxTimestep > RecMaxWidth/8 {
		maxTimestep = RecMaxWidth / 8
	}

	text := ctcGreedyDecode(outData, maxTimestep, EnRecVocabSize, e.enDictList)

	var totalConf float32
	for t := 0; t < maxTimestep; t++ {
		offset := t * EnRecVocabSize
		maxVal := outData[offset]
		for c := 1; c < EnRecVocabSize; c++ {
			if outData[offset+c] > maxVal {
				maxVal = outData[offset+c]
			}
		}
		totalConf += maxVal
	}
	confidence := totalConf / float32(maxTimestep)

	return &OCRLine{
		Text:       fixEnglishOCRErrors(text),
		BBox:       box,
		Confidence: confidence,
	}, nil
}

// ctcGreedyDecode performs greedy CTC decoding on the recognition output.
// It takes the argmax at each timestep, collapses consecutive identical non-blank labels,
// and removes blank labels (index 0).
// vocab is the total vocabulary size (including blank at index 0).
func ctcGreedyDecode(logits []float32, timesteps, vocab int, dict []string) string {
	const blankLabel = 0

	var decoded []int
	var prevLabel = blankLabel
	for t := 0; t < timesteps; t++ {
		offset := t * vocab
		// Argmax
		maxIdx := 0
		maxVal := logits[offset]
		for c := 1; c < vocab; c++ {
			if logits[offset+c] > maxVal {
				maxVal = logits[offset+c]
				maxIdx = c
			}
		}

		if maxIdx != blankLabel && maxIdx != prevLabel {
			decoded = append(decoded, maxIdx)
		}
		prevLabel = maxIdx
	}

	// Map label indices to characters using the loaded dictionary or built-in mapping
	return decodeLabels(decoded, dict)
}

// decodeLabels maps integer label indices to characters using the provided
// dictionary (loaded from dict.txt). Falls back to built-in ASCII/CJK mapping
// when dict is empty.
func decodeLabels(labels []int, dict []string) string {
	var result []rune
	for _, idx := range labels {
		if idx <= 0 {
			continue
		}
		// Try dictionary first (index 1-based)
		if idx <= len(dict) {
			for _, r := range dict[idx-1] {
				result = append(result, r)
			}
			continue
		}
		// Index 96 is used by the English model as a space/word separator.
		// Only treat it as space when a dictionary is loaded (indicating English mode).
		if idx == 96 && len(dict) > 0 {
			result = append(result, ' ')
			continue
		}
		// When no dict loaded, fall back to ASCII/CJK mapping
		if len(dict) == 0 {
			if idx <= 94 {
				result = append(result, rune(0x20+idx))
			} else {
				result = append(result, rune(0x4E00+(idx-95)))
			}
			continue
		}
		// Dict loaded but index beyond range: skip
	}
	return string(result)
}
