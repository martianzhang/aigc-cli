package ocr

import (
	"fmt"
	"image"
)

// Detect runs text detection on the image and returns text region bounding boxes.
// Each box is a four-point polygon [top-left, top-right, bottom-right, bottom-left].
// Automatically selects the optimal input resolution based on image size and
// the configured maxSide. Re-creates the ONNX session when a different size
// is needed (lazy, cached across images of similar dimensions).
func (e *Engine) Detect(img image.Image) ([][4][2]int, error) {
	b := img.Bounds()
	origW := b.Dx()
	origH := b.Dy()

	// Compute the required input size for this image.
	inputSize := e.detInputSizeFor(origW, origH)

	// Lazy re-init: if the session doesn't exist or needs a different size,
	// re-create the detection session with the correct input shape.
	if e.detInputSize != inputSize {
		if err := e.reinitDet(inputSize); err != nil {
			return nil, fmt.Errorf("reinit det session: %w", err)
		}
	}

	// Preprocess with the chosen input size.
	pixels, scaleX, scaleY, padLeft, padTop := detPreprocess(img, inputSize)

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

	// Read output probability map
	outData := e.detOut.GetData()
	outH := inputSize
	outW := inputSize

	// Post-process: DB binarization → connected components → NMS
	boxes := detPostProcess(outData, outH, outW, inputSize, scaleX, scaleY, padLeft, padTop, origW, origH)

	return boxes, nil
}
