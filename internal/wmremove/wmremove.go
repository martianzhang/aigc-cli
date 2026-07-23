package wmremove

import (
	"fmt"
	"image"
	"image/png"
	"os"

	ort "github.com/amikos-tech/pure-onnx/ort"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

const (
	ImageInput = "image"
	MaskInput  = "mask"
	OutputName = "result"
)

type Detector struct {
	modelPath string
	libPath   string
}

func NewDetector(libPath, modelPath string) (*Detector, error) {
	if _, err := os.Stat(libPath); err != nil {
		return nil, fmt.Errorf("onnx runtime not found: %w", err)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}
	d := &Detector{libPath: libPath, modelPath: modelPath}
	if err := d.initEnv(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *Detector) initEnv() error {
	if err := ort.SetSharedLibraryPath(d.libPath); err != nil {
		return err
	}
	_ = ort.SetLogLevel(ort.LoggingLevelError)
	return ort.InitializeEnvironment()
}

// RemoveWatermark runs MI-GAN with the given image and mask.
// mask: white(255)=inpaint area, black(0)=keep. Model handles resize internally.
func (d *Detector) RemoveWatermark(img image.Image, mask *image.Gray) (*image.NRGBA, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	imgData := preprocessImage(img, w, h)
	maskData := preprocessMask(mask, w, h)

	imgShape := ort.NewShape(1, 3, int64(h), int64(w))
	maskShape := ort.NewShape(1, 1, int64(h), int64(w))
	outShape := ort.NewShape(1, 3, int64(h), int64(w))

	imgTensor, err := ort.NewTensor(imgShape, imgData)
	if err != nil {
		return nil, fmt.Errorf("create image tensor: %w", err)
	}
	defer imgTensor.Destroy()

	maskTensor, err := ort.NewTensor(maskShape, maskData)
	if err != nil {
		return nil, fmt.Errorf("create mask tensor: %w", err)
	}
	defer maskTensor.Destroy()

	outData := make([]uint8, 3*w*h)
	outTensor, err := ort.NewTensor(outShape, outData)
	if err != nil {
		return nil, fmt.Errorf("create output tensor: %w", err)
	}
	defer outTensor.Destroy()

	opts := ort.NewCUDASessionOptions()
	session, err := ort.NewAdvancedSession(
		d.modelPath,
		[]string{ImageInput, MaskInput},
		[]string{OutputName},
		[]ort.Value{imgTensor, maskTensor},
		[]ort.Value{outTensor},
		opts,
	)
	if opts != nil {
		opts.Destroy()
	}
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	defer session.Destroy()

	if err := session.Run(); err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	result := outTensor.GetData()
	out := compositeResult(img, mask, result, w, h)
	return out, nil
}

func compositeResult(img image.Image, mask *image.Gray, result []uint8, w, h int) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(b)
	softMask := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mask.GrayAt(x, y).Y > 128 {
				softMask[y*w+x] = 1.0
			}
		}
	}
	for iter := 0; iter < 3; iter++ {
		tmp := make([]float64, w*h)
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				s := softMask[(y-1)*w+(x-1)] + softMask[(y-1)*w+x] + softMask[(y-1)*w+(x+1)] +
					softMask[y*w+(x-1)] + softMask[y*w+x] + softMask[y*w+(x+1)] +
					softMask[(y+1)*w+(x-1)] + softMask[(y+1)*w+x] + softMask[(y+1)*w+(x+1)]
				tmp[y*w+x] = s / 9.0
			}
		}
		softMask = tmp
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			di := y*w*4 + x*4
			f := softMask[y*w+x]
			si := y*w + x
			r, g, b, _ := img.At(x, y).RGBA()
			origR, origG, origB := float64(r>>8), float64(g>>8), float64(b>>8)
			aiR, aiG, aiB := float64(result[si]), float64(result[si+w*h]), float64(result[si+2*w*h])
			out.Pix[di] = uint8(aiR*f + origR*(1-f))
			out.Pix[di+1] = uint8(aiG*f + origG*(1-f))
			out.Pix[di+2] = uint8(aiB*f + origB*(1-f))
			out.Pix[di+3] = 255
		}
	}
	out.Stride = w * 4
	return out
}

func (d *Detector) Close() {
	ort.DestroyEnvironment()
}

func SavePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func DefaultLibPath(modelsDir string) (string, error) {
	return onnxrt.LibPath(modelsDir)
}

func DefaultModelPath(modelsDir string) string {
	return modelsDir + "/migan.onnx"
}
