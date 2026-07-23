package ocr

import (
	"fmt"
	"image"
	"math"

	"golang.org/x/image/draw"
)

// clsInputW and clsInputH are the fixed input dimensions for the direction classifier.
const (
	clsInputH = 48
	clsInputW = 192
)

// classifyDirection checks if a text region is upside-down using the direction classifier.
// Returns true if the image should be rotated 180° before recognition.
// When the classifier isn't loaded or confidence is low, returns false (pass-through).
func (e *Engine) classifyDirection(cropped *image.RGBA) bool {
	if e.cls == nil || cropped == nil {
		return false
	}
	b := cropped.Bounds()
	if b.Dx() < 4 || b.Dy() < 4 {
		return false
	}

	// Resize to 48x192 (maintain aspect ratio, center-pad).
	resized := image.NewRGBA(image.Rect(0, 0, clsInputW, clsInputH))
	draw.BiLinear.Scale(resized, resized.Bounds(), cropped, b, draw.Src, nil)

	// Build normalized tensor: (pixel/255 - 0.5) / 0.5 → [-1, 1]
	data := e.clsIn.GetData()
	idx := 0
	for c := 0; c < DetChannels; c++ {
		for y := 0; y < clsInputH; y++ {
			for x := 0; x < clsInputW; x++ {
				r, g, b_, _ := resized.At(x, y).RGBA()
				var val float32
				switch c {
				case 0:
					val = float32(r) / 65535.0
				case 1:
					val = float32(g) / 65535.0
				case 2:
					val = float32(b_) / 65535.0
				}
				data[idx] = (val - 0.5) / 0.5
				idx++
			}
		}
	}

	if err := e.cls.Run(); err != nil {
		return false
	}

	out := e.clsOut.GetData()
	if len(out) < 2 {
		return false
	}
	// Softmax over 2 classes: class 0 = 0°, class 1 = 180°.
	// If 180° confidence is above threshold, rotate.
	maxIdx := 0
	if out[1] > out[0] {
		maxIdx = 1
	}
	// Softmax normalization for confidence scoring.
	exp0 := math.Exp(float64(out[0]))
	exp1 := math.Exp(float64(out[1]))
	sum := exp0 + exp1
	if sum <= 0 {
		return false
	}
	conf180 := exp1 / sum
	return maxIdx == 1 && conf180 > 0.7
}

// Recognize recognizes text in a single image region using Chinese model.
// Direction classification is applied automatically before recognition when the
// classifier model is loaded.
func (e *Engine) Recognize(img image.Image, box [4][2]int) (*OCRLine, error) {
	// Preprocess (includes affine correction + direction classification)
	pixels, regionWidth := recPreprocess(e, img, box)
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

	pixels, regionWidth := recPreprocess(e, img, box)
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
		Text:       e.fixEnglishOCRErrors(text),
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
