package face

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"

	pigo "github.com/esimov/pigo/core"
)

const (
	perturb       = 63
	minFaceSize   = 20
	maxFaceSize   = 1000
	qThreshold    = 5.0
	shiftFactor   = 0.10
	scaleFactor   = 1.15
	NumLandmarks  = 15
	NumEyePoints  = 5
	NumMouthPoint = 4
)

var (
	eyeCascades   = [5]string{"lp46", "lp44", "lp42", "lp38", "lp312"}
	mouthCascades = [4]string{"lp93", "lp84", "lp82", "lp81"}
)

// Face 是一张人脸的检测结果（原图像素坐标）。
// Landmarks 固定 15 槽位：[0-4]左眼区 5 点, [5-9]右眼区 5 点, [10-13]嘴 4 点, [14]鼻。
// 未检出的槽位为零值（{0,0}）。
type Face struct {
	Box       [4]float32
	Score     float32
	LeftEye   [2]float32
	RightEye  [2]float32
	Landmarks [NumLandmarks][2]float32
}

// Detector 用 pigo（纯 Go Pico 算法）检测人脸。级联文件从 models/face/ 目录读取。
type Detector struct {
	classifier *pigo.Pigo
	puploc     *pigo.PuplocCascade
	flpcs      map[string][]*pigo.FlpCascade
}

// NewDetector 从 modelsDir/face/ 加载人脸检测/瞳孔/关键点级联。
func NewDetector(modelsDir string) (*Detector, error) {
	faceDir := filepath.Join(modelsDir, "face")

	facefinder, err := os.ReadFile(filepath.Join(faceDir, "facefinder"))
	if err != nil {
		return nil, fmt.Errorf("read facefinder cascade: %w\n  Run: aigc-cli depth init --face", err)
	}
	classifier, err := pigo.NewPigo().Unpack(facefinder)
	if err != nil {
		return nil, fmt.Errorf("unpack facefinder cascade: %w", err)
	}

	puplocData, err := os.ReadFile(filepath.Join(faceDir, "puploc"))
	if err != nil {
		return nil, fmt.Errorf("read puploc cascade: %w", err)
	}
	puploc, err := pigo.NewPuplocCascade().UnpackCascade(puplocData)
	if err != nil {
		return nil, fmt.Errorf("unpack puploc cascade: %w", err)
	}

	flpcs := make(map[string][]*pigo.FlpCascade)
	for _, name := range append(eyeCascades[:], mouthCascades[:]...) {
		data, err := os.ReadFile(filepath.Join(faceDir, "lps", name))
		if err != nil {
			return nil, fmt.Errorf("read lp cascade %s: %w", name, err)
		}
		flpc, err := pigo.NewPuplocCascade().UnpackCascade(data)
		if err != nil {
			return nil, fmt.Errorf("unpack lp cascade %s: %w", name, err)
		}
		flpcs[name] = []*pigo.FlpCascade{{PuplocCascade: flpc}}
	}
	// lp84 额外以镜像方式定位鼻子（键已存在，追加第二个实例）。
	lp84, err := os.ReadFile(filepath.Join(faceDir, "lps", "lp84"))
	if err != nil {
		return nil, fmt.Errorf("read lp84 cascade: %w", err)
	}
	flpc84, err := pigo.NewPuplocCascade().UnpackCascade(lp84)
	if err != nil {
		return nil, fmt.Errorf("unpack lp84 cascade: %w", err)
	}
	flpcs["lp84"] = append(flpcs["lp84"], &pigo.FlpCascade{PuplocCascade: flpc84})

	return &Detector{classifier: classifier, puploc: puploc, flpcs: flpcs}, nil
}

// decodeBox 把 pigo 检测结果（中心行列 + 正方形边长）映射为 [x1,y1,x2,y2]。
func decodeBox(d pigo.Detection) [4]float32 {
	return [4]float32{
		float32(d.Col - d.Scale/2),
		float32(d.Row - d.Scale/2),
		float32(d.Col + d.Scale/2),
		float32(d.Row + d.Scale/2),
	}
}

// Detect 检测图片中的人脸，返回原图像素坐标的检测框、瞳孔与 15 点关键点。
func (d *Detector) Detect(img image.Image) ([]Face, error) {
	src := pigo.ImgToNRGBA(img)
	pixels := pigo.RgbToGrayscale(src)
	cols, rows := src.Bounds().Dx(), src.Bounds().Dy()

	imgParams := &pigo.ImageParams{Pixels: pixels, Rows: rows, Cols: cols, Dim: cols}
	cParams := pigo.CascadeParams{
		MinSize:     minFaceSize,
		MaxSize:     maxFaceSize,
		ShiftFactor: shiftFactor,
		ScaleFactor: scaleFactor,
		ImageParams: *imgParams,
	}

	dets := d.classifier.RunCascade(cParams, 0.0)
	dets = d.classifier.ClusterDetections(dets, 0.15)
	if len(dets) == 0 {
		return nil, nil
	}

	var faces []Face
	for _, det := range dets {
		if det.Q <= qThreshold {
			continue
		}
		f := Face{
			Box:   decodeBox(det),
			Score: det.Q,
		}

		if d.puploc != nil && det.Scale > 50 {
			leftEye := d.puploc.RunDetector(pigo.Puploc{
				Row:      det.Row - int(0.075*float32(det.Scale)),
				Col:      det.Col - int(0.175*float32(det.Scale)),
				Scale:    float32(det.Scale) * 0.25,
				Perturbs: perturb,
			}, *imgParams, 0.0, false)
			rightEye := d.puploc.RunDetector(pigo.Puploc{
				Row:      det.Row - int(0.075*float32(det.Scale)),
				Col:      det.Col + int(0.185*float32(det.Scale)),
				Scale:    float32(det.Scale) * 0.25,
				Perturbs: perturb,
			}, *imgParams, 0.0, false)

			if leftEye.Row > 0 && leftEye.Col > 0 {
				f.LeftEye = [2]float32{float32(leftEye.Col), float32(leftEye.Row)}
			}
			if rightEye.Row > 0 && rightEye.Col > 0 {
				f.RightEye = [2]float32{float32(rightEye.Col), float32(rightEye.Row)}
			}

			if len(d.flpcs) > 0 && f.LeftEye != [2]float32{} && f.RightEye != [2]float32{} {
				d.collectLandmarks(imgParams, &f, leftEye, rightEye)
			}
		}
		faces = append(faces, f)
	}
	sort.Slice(faces, func(i, j int) bool { return faces[i].Score > faces[j].Score })
	return faces, nil
}

// collectLandmarks 用 15 个稀疏关键点级联定位五官点，按固定槽位填充。
// 槽位：0-4 左眼区、5-9 右眼区、10-13 嘴、14 鼻。未检出的槽位保持零值。
func (d *Detector) collectLandmarks(img *pigo.ImageParams, f *Face, leftEye, rightEye *pigo.Puploc) {
	for i, eye := range eyeCascades {
		for _, flpc := range d.flpcs[eye] {
			if flp := flpc.GetLandmarkPoint(leftEye, rightEye, *img, perturb, false); flp.Row > 0 && flp.Col > 0 {
				f.Landmarks[i] = [2]float32{float32(flp.Col), float32(flp.Row)}
			}
			if flp := flpc.GetLandmarkPoint(leftEye, rightEye, *img, perturb, true); flp.Row > 0 && flp.Col > 0 {
				f.Landmarks[NumEyePoints+i] = [2]float32{float32(flp.Col), float32(flp.Row)}
			}
		}
	}
	for i, mouth := range mouthCascades {
		for _, flpc := range d.flpcs[mouth] {
			if flp := flpc.GetLandmarkPoint(leftEye, rightEye, *img, perturb, false); flp.Row > 0 && flp.Col > 0 {
				f.Landmarks[2*NumEyePoints+i] = [2]float32{float32(flp.Col), float32(flp.Row)}
			}
		}
	}
	if nose := d.flpcs["lp84"][0].GetLandmarkPoint(leftEye, rightEye, *img, perturb, true); nose.Row > 0 && nose.Col > 0 {
		f.Landmarks[2*NumEyePoints+NumMouthPoint] = [2]float32{float32(nose.Col), float32(nose.Row)}
	}
}
