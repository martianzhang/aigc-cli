package skeleton

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"sort"

	ort "github.com/amikos-tech/pure-onnx/ort"
	"golang.org/x/image/draw"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
)

// Model 常量（与模型文件匹配）。
const (
	ModelInputSize = 640
	ModelChannels  = 3
	// NumAnchors 是 YOLOv8n-pose 的输出 anchor 数。
	NumAnchors = 8400
	// OutChannels = 4(box xywh) + 1(conf) + 17*3(keypoints)。
	OutChannels = 56
)

// Detector 管理 YOLOv8n-pose 的 ONNX 会话。
type Detector struct {
	session *ort.AdvancedSession
	input   *ort.Tensor[float32]
	output  *ort.Tensor[float32]
}

// NewDetector 创建 YOLOv8n-pose Detector。
func NewDetector(libPath, modelPath string) (*Detector, error) {
	if _, err := os.Stat(libPath); err != nil {
		return nil, fmt.Errorf("onnx runtime library not found: %w", err)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}

	if err := onnxrt.InitEnvironment(libPath); err != nil {
		return nil, err
	}

	d := &Detector{}
	// 输入 [1,3,640,640]
	inShape := ort.NewShape(1, ModelChannels, ModelInputSize, ModelInputSize)
	inData := make([]float32, 1*ModelChannels*ModelInputSize*ModelInputSize)
	var err error
	d.input, err = ort.NewTensor(inShape, inData)
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create input tensor: %w", err)
	}
	// 输出 [1,56,8400]
	outShape := ort.NewShape(1, OutChannels, NumAnchors)
	outData := make([]float32, 1*OutChannels*NumAnchors)
	d.output, err = ort.NewTensor(outShape, outData)
	if err != nil {
		d.input.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create output tensor: %w", err)
	}

	opts := ort.NewCUDASessionOptions()
	if opts == nil {
		opts, err = ort.NewSessionOptions()
		if err == nil {
			_ = opts.SetIntraOpNumThreads(4)
			_ = opts.AddConfigEntry("mlas.disable_kleidiai", "1")
		}
	}
	d.session, err = ort.NewAdvancedSession(
		modelPath,
		[]string{"images"},
		[]string{"output0"},
		[]ort.Value{d.input},
		[]ort.Value{d.output},
		opts,
	)
	if opts != nil {
		opts.Destroy()
	}
	if err != nil {
		d.output.Destroy()
		d.input.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create session: %w", err)
	}
	return d, nil
}

// Detect 检测图片中的人体姿态，返回人物列表（原图像素坐标）。
func (d *Detector) Detect(img image.Image, confThresh float32) ([]Person, error) {
	b := img.Bounds()
	origW := b.Dx()
	origH := b.Dy()

	// letterbox 到 640×640
	pixels, scale, padX, padY := preprocess(img)

	data := d.input.GetData()
	if len(data) != len(pixels) {
		return nil, fmt.Errorf("tensor size mismatch")
	}
	copy(data, pixels)

	if err := d.session.Run(); err != nil {
		return nil, fmt.Errorf("inference failed: %w", err)
	}

	out := d.output.GetData() // [56, 8400] 展平
	return postprocess(out, origW, origH, scale, padX, padY, confThresh), nil
}

// preprocess 返回归一化像素 + letterbox 缩放/偏移。
func preprocess(img image.Image) ([]float32, float32, int, int) {
	srcW := img.Bounds().Dx()
	srcH := img.Bounds().Dy()
	scale := float32(ModelInputSize) / float32(max(srcW, srcH))
	nw := int(float32(srcW) * scale)
	nh := int(float32(srcH) * scale)
	padX := (ModelInputSize - nw) / 2
	padY := (ModelInputSize - nh) / 2

	// 双线性缩放原图到 nw×nh
	resized := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.BiLinear.Scale(resized, resized.Bounds(), img, img.Bounds(), draw.Src, nil)

	// 铺到 640×640 canvas（灰边 114）
	canvas := image.NewRGBA(image.Rect(0, 0, ModelInputSize, ModelInputSize))
	for i := range canvas.Pix {
		canvas.Pix[i] = 114
	}
	draw.Draw(canvas, image.Rect(padX, padY, padX+nw, padY+nh), resized, image.Point{}, draw.Src)

	pixels := make([]float32, ModelChannels*ModelInputSize*ModelInputSize)
	idx := 0
	for c := 0; c < ModelChannels; c++ {
		for y := 0; y < ModelInputSize; y++ {
			for x := 0; x < ModelInputSize; x++ {
				off := canvas.PixOffset(x, y)
				var v uint8
				switch c {
				case 0:
					v = canvas.Pix[off]
				case 1:
					v = canvas.Pix[off+1]
				case 2:
					v = canvas.Pix[off+2]
				}
				pixels[idx] = float32(v) / 255.0
				idx++
			}
		}
	}
	return pixels, scale, padX, padY
}

// postprocess 解析输出、NMS、坐标映射回原图。
// 输出 [1, 56, 8400] row-major 展平：idx = channel*8400 + anchor。
func postprocess(out []float32, origW, origH int, scale float32, padX, padY int, confThresh float32) []Person {
	type det struct {
		person Person
		score  float32
	}
	var dets []det

	for a := 0; a < NumAnchors; a++ {
		// 通道优先布局：channel c 的值在 out[c*8400 + a]
		conf := out[4*NumAnchors+a]
		if conf < confThresh {
			continue
		}
		// box xywh（640 坐标）
		cx := out[0*NumAnchors+a] - float32(padX)
		cy := out[1*NumAnchors+a] - float32(padY)
		bw := out[2*NumAnchors+a]
		bh := out[3*NumAnchors+a]
		p := Person{Score: conf}
		p.Box = [4]float32{
			(cx - bw/2) / scale, (cy - bh/2) / scale,
			(cx + bw/2) / scale, (cy + bh/2) / scale,
		}
		for k := 0; k < 17; k++ {
			p.Keypoints[k] = [3]float32{
				(out[(5+k*3)*NumAnchors+a] - float32(padX)) / scale,
				(out[(6+k*3)*NumAnchors+a] - float32(padY)) / scale,
				out[(7+k*3)*NumAnchors+a],
			}
		}
		dets = append(dets, det{p, conf})
	}

	// 按置信度降序
	sort.Slice(dets, func(i, j int) bool { return dets[i].score > dets[j].score })

	// NMS (IoU 0.45)
	kept := make([]bool, len(dets))
	var people []Person
	for i := range dets {
		if kept[i] {
			continue
		}
		people = append(people, dets[i].person)
		for j := i + 1; j < len(dets); j++ {
			if !kept[j] && iou(dets[i].person.Box, dets[j].person.Box) > 0.45 {
				kept[j] = true
			}
		}
	}
	return people
}

// iou 计算两个 box 的交并比。
func iou(a, b [4]float32) float32 {
	x1, y1 := maxf(a[0], b[0]), maxf(a[1], b[1])
	x2, y2 := minf(a[2], b[2]), minf(a[3], b[3])
	interW, interH := x2-x1, y2-y1
	if interW <= 0 || interH <= 0 {
		return 0
	}
	inter := interW * interH
	areaA := (a[2] - a[0]) * (a[3] - a[1])
	areaB := (b[2] - b[0]) * (b[3] - b[1])
	return inter / (areaA + areaB - inter)
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Close 释放资源。
func (d *Detector) Close() {
	if d.session != nil {
		d.session.Destroy()
	}
	if d.output != nil {
		d.output.Destroy()
	}
	if d.input != nil {
		d.input.Destroy()
	}
	// ONNX Runtime 环境是进程级单例（ortEnvOnce 初始化），由所有
	// Detector 共享；这里不销毁环境，避免其他 session 崩溃。
}
