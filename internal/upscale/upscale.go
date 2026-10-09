// Package upscale provides local AI image super-resolution using Real-ESRGAN /
// Real-CUGAN / Swin2SR ONNX models, run through the shared pure-Go ONNX Runtime.
//
// Large images are processed in overlapping tiles that are feathered back
// together, mirroring the reference browser implementation, so memory stays
// bounded regardless of input size.
package upscale

import (
	"fmt"
	"image"
	"os"

	ort "github.com/amikos-tech/pure-onnx/ort"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

// Tensor names for the Real-ESRGAN-family ONNX exports.
const (
	InputName  = "input"
	OutputName = "output"
)

const (
	tileSize    = 256
	tileOverlap = 16

	maxOutputDim    = 16384
	maxOutputPixels = 200_000_000
)

// Detector runs a super-resolution model on the shared ONNX Runtime environment.
// The session is built once for a fixed tile and reused across all tiles/images.
type Detector struct {
	libPath   string
	modelPath string
	scale     int
	inName    string
	outName   string
	session   *ort.AdvancedSession
	input     *ort.Tensor[float32]
	output    *ort.Tensor[float32]
}

// NewDetector initializes the ONNX Runtime environment and the fixed-size
// inference session. scale is the model's native upscale factor (2 or 4);
// inName/outName are the model's input/output tensor names.
func NewDetector(libPath, modelPath string, scale int, inName, outName string) (*Detector, error) {
	if _, err := os.Stat(libPath); err != nil {
		return nil, fmt.Errorf("onnx runtime library not found: %w", err)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}
	if scale <= 0 {
		scale = 4
	}
	if inName == "" {
		inName = InputName
	}
	if outName == "" {
		outName = OutputName
	}
	if err := onnxrt.InitEnvironment(libPath); err != nil {
		return nil, err
	}
	d := &Detector{libPath: libPath, modelPath: modelPath, scale: scale, inName: inName, outName: outName}
	if err := d.init(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Detector) init() error {
	inData := make([]float32, 3*tileSize*tileSize)
	input, err := ort.NewTensor(ort.NewShape(1, 3, tileSize, tileSize), inData)
	if err != nil {
		return fmt.Errorf("create input tensor: %w", err)
	}
	d.input = input

	outSide := tileSize * d.scale
	outData := make([]float32, 3*outSide*outSide)
	output, err := ort.NewTensor(ort.NewShape(1, 3, int64(outSide), int64(outSide)), outData)
	if err != nil {
		_ = input.Destroy()
		return fmt.Errorf("create output tensor: %w", err)
	}
	d.output = output

	opts, err := ort.NewSessionOptions()
	if err != nil {
		_ = output.Destroy()
		_ = input.Destroy()
		return fmt.Errorf("create session options: %w", err)
	}
	d.session, err = ort.NewAdvancedSession(
		d.modelPath,
		[]string{d.inName},
		[]string{d.outName},
		[]ort.Value{d.input},
		[]ort.Value{d.output},
		opts,
	)
	opts.Destroy()
	if err != nil {
		_ = output.Destroy()
		_ = input.Destroy()
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// ModelPath returns the ONNX model path in use.
func (d *Detector) ModelPath() string { return d.modelPath }

// Scale returns the model's native upscale factor.
func (d *Detector) Scale() int { return d.scale }

// Upscale runs the model over img and returns an NRGBA image whose width and
// height are multiplied by the model's native scale.
func (d *Detector) Upscale(img image.Image) (*image.NRGBA, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil, fmt.Errorf("empty image")
	}
	outW, outH := w*d.scale, h*d.scale
	if outW > maxOutputDim || outH > maxOutputDim || outW*outH > maxOutputPixels {
		return nil, fmt.Errorf("output %dx%d exceeds the %dpx / %.0fMP limit; use a smaller --scale",
			outW, outH, maxOutputDim, float64(maxOutputPixels)/1e6)
	}

	dst := image.NewNRGBA(image.Rect(0, 0, outW, outH))
	full, ramp := BlendWeights(tileSize, d.scale, tileOverlap)
	step := tileSize - tileOverlap
	tilesX, tilesY := tileCount(w, step), tileCount(h, step)

	tileBuf := make([]float32, 3*tileSize*tileSize)
	inData := d.input.GetData()
	for ty := 0; ty < tilesY; ty++ {
		ay := full
		if ty != 0 {
			ay = ramp
		}
		for tx := 0; tx < tilesX; tx++ {
			ax := full
			if tx != 0 {
				ax = ramp
			}
			x0, y0 := tx*step, ty*step
			ExtractTile(img, x0, y0, tileSize, tileBuf)
			copy(inData, tileBuf)
			if err := d.session.Run(); err != nil {
				return nil, fmt.Errorf("inference failed: %w", err)
			}
			BlendTile(dst, d.output.GetData(), x0, y0, outW, outH, d.scale, ax, ay)
		}
	}
	return dst, nil
}

// Close releases the session and tensors (the shared environment is left intact).
func (d *Detector) Close() {
	if d.session != nil {
		_ = d.session.Destroy()
	}
	if d.output != nil {
		_ = d.output.Destroy()
	}
	if d.input != nil {
		_ = d.input.Destroy()
	}
}

func tileCount(n, step int) int {
	c := (n - tileOverlap + step - 1) / step
	if c < 1 {
		return 1
	}
	return c
}
