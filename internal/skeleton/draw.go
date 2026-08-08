// Package skeleton 提供基于 YOLOv8n-pose ONNX 模型的人体姿态估计（COCO 17 关键点）。
//
// 整体流程：
//
//	图片 → letterbox 640×640 → /255 归一化 NCHW
//	       → ONNX 推理 (pure-onnx)
//	       → 输出 [1, 56, 8400] = 4(box xywh) + 1(conf) + 51(17×3 keypoints)
//	       → NMS 过滤 → 坐标映射回原图 → 绘制骨架
package skeleton

import (
	"image"
	"image/color"
)

// COCO17 是 17 个人体关键点名称（索引即关键点 ID）。
var COCO17 = []string{
	"nose", "left_eye", "right_eye", "left_ear", "right_ear",
	"left_shoulder", "right_shoulder",
	"left_elbow", "right_elbow",
	"left_wrist", "right_wrist",
	"left_hip", "right_hip",
	"left_knee", "right_knee",
	"left_ankle", "right_ankle",
}

// SkeletonEdges 是 19 条骨架连线（[from, to]，COCO 17 索引）。
var SkeletonEdges = [][2]int{
	{15, 13}, {13, 11}, {16, 14}, {14, 12}, // 腿
	{11, 12}, {5, 11}, {6, 12}, {5, 6}, // 骨盆 + 躯干 + 肩
	{5, 7}, {6, 8}, {7, 9}, {8, 10}, // 臂
	{1, 2}, {0, 1}, {0, 2}, {1, 3}, {2, 4}, // 眼/鼻
	{3, 5}, {4, 6}, // 耳→肩
}

// Person 是一个人的检测结果。
type Person struct {
	// Box 是检测框（原图像素坐标，x1,y1,x2,y2）。
	Box [4]float32
	// Score 是检测置信度。
	Score float32
	// Keypoints 是 17 个关键点（原图像素坐标 x,y + 可见度）。
	Keypoints [17][3]float32
}

// DrawSkeleton 把骨架绘制到图片上。
func DrawSkeleton(dst *image.RGBA, people []Person) {
	boneColor := color.RGBA{R: 0, G: 255, B: 0, A: 255}  // 绿色骨架
	jointColor := color.RGBA{R: 255, G: 0, B: 0, A: 255} // 红色关节点

	for _, p := range people {
		// 骨架线
		for _, e := range SkeletonEdges {
			a, b := p.Keypoints[e[0]], p.Keypoints[e[1]]
			if a[2] < 0.3 || b[2] < 0.3 {
				continue
			}
			line(dst, int(a[0]), int(a[1]), int(b[0]), int(b[1]), boneColor)
		}
		// 关节点
		for _, k := range p.Keypoints {
			if k[2] < 0.3 {
				continue
			}
			dot(dst, int(k[0]), int(k[1]), jointColor)
		}
	}
}

// line 用 Bresenham 画粗线（3px 宽）。
func line(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		// 3×3 笔刷使线更清晰
		for by := -1; by <= 1; by++ {
			for bx := -1; bx <= 1; bx++ {
				px, py := x0+bx, y0+by
				if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
					img.Set(px, py, c)
				}
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

// dot 画一个实心圆点（半径 3）。
func dot(img *image.RGBA, x, y int, c color.RGBA) {
	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			if dx*dx+dy*dy <= 9 {
				px, py := x+dx, y+dy
				if px >= 0 && py >= 0 && px < img.Bounds().Dx() && py < img.Bounds().Dy() {
					img.Set(px, py, c)
				}
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
