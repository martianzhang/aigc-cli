package ocr

import (
	"fmt"
	"image"
)

// Detect runs text detection on the image and returns text region bounding boxes.
// Each box is a four-point polygon [top-left, top-right, bottom-right, bottom-left].
func (e *Engine) Detect(img image.Image) ([][4][2]int, error) {
	b := img.Bounds()
	origW := b.Dx()
	origH := b.Dy()

	// Preprocess
	pixels, scaleX, scaleY, padLeft, padTop := detPreprocess(img)

	// Copy to input tensor
	data := e.detIn.GetData()
	if len(data) != len(pixels) {
		return nil, fmt.Errorf("det input tensor size mismatch: got %d want %d", len(data), len(pixels))
	}
	copy(data, pixels)

	// Run inference
	if err := e.det.Run(); err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	// Read output probability map (full resolution: 960x960)
	outData := e.detOut.GetData()
	outH := DetInputSize
	outW := DetInputSize

	// Post-process: DB binarization → connected components → NMS
	boxes := detPostProcess(outData, outH, outW, scaleX, scaleY, padLeft, padTop, origW, origH)

	return boxes, nil
}
