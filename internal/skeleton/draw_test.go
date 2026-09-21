package skeleton

import (
	"image"
	"image/color"
	"testing"
)

// 与 draw.go 中 DrawSkeleton 使用的颜色一致：绿色骨骼 + 红色关节点。
var (
	testBoneColor  = color.RGBA{R: 0, G: 255, B: 0, A: 255}
	testJointColor = color.RGBA{R: 255, G: 0, B: 0, A: 255}
	testEmptyPixel = color.RGBA{}
)

// assertPixel 校验指定像素颜色，失败时输出坐标便于定位。
func assertPixel(t *testing.T, img *image.RGBA, x, y int, want color.RGBA) {
	t.Helper()
	if got := img.RGBAAt(x, y); got != want {
		t.Errorf("pixel(%d,%d) = %v, want %v", x, y, got, want)
	}
}

// countDrawn 统计非空像素数量。
func countDrawn(img *image.RGBA) int {
	n := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y) != testEmptyPixel {
				n++
			}
		}
	}
	return n
}

func TestAbs(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"positive", 7, 7},
		{"negative", -7, 7},
		{"zero", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := abs(tt.in); got != tt.want {
				t.Errorf("abs(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestDot(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	assertPixel(t, img, 50, 50, testEmptyPixel) // 绘制前应为空

	dot(img, 50, 50, testJointColor)

	// 半径 3 内（dx²+dy²<=9）应为关节色
	inside := []struct{ x, y int }{
		{50, 50}, {53, 50}, {47, 50}, {50, 53}, {50, 47}, // 圆心 + 四向
		{52, 52}, {48, 52}, {52, 48}, {48, 48}, // 四个对角
	}
	for _, p := range inside {
		assertPixel(t, img, p.x, p.y, testJointColor)
	}

	// 半径外（dx²+dy²>9）不应被绘制
	outside := []struct{ x, y int }{
		{54, 50}, {46, 50}, {50, 54}, {50, 46},
		{53, 53}, {47, 53}, {53, 47}, {47, 47},
	}
	for _, p := range outside {
		assertPixel(t, img, p.x, p.y, testEmptyPixel)
	}
}

func TestDotClippedAtBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	dot(img, 0, 0, testJointColor) // 圆心在左上角，越界像素必须跳过

	assertPixel(t, img, 0, 0, testJointColor)
	assertPixel(t, img, 3, 0, testJointColor)
	assertPixel(t, img, 4, 0, testEmptyPixel)
	assertPixel(t, img, 99, 99, testEmptyPixel) // 不得因负坐标回绕到图像另一端
}

func TestLineHorizontal(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	assertPixel(t, img, 10, 20, testEmptyPixel)
	assertPixel(t, img, 30, 20, testEmptyPixel)

	line(img, 10, 20, 30, 20, testBoneColor)

	// 起止端点必须被绘制
	assertPixel(t, img, 10, 20, testBoneColor)
	assertPixel(t, img, 30, 20, testBoneColor)
	// 3×3 笔刷：线上及上下各一行均为骨骼色
	for x := 10; x <= 30; x++ {
		for y := 19; y <= 21; y++ {
			assertPixel(t, img, x, y, testBoneColor)
		}
	}
	// 端点与笔刷之外不应被绘制
	assertPixel(t, img, 8, 20, testEmptyPixel)
	assertPixel(t, img, 32, 20, testEmptyPixel)
	assertPixel(t, img, 20, 18, testEmptyPixel)
	assertPixel(t, img, 20, 22, testEmptyPixel)
}

func TestLineDiagonalReverse(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// 反向斜线，覆盖 sx=-1 / sy=-1 分支
	line(img, 80, 80, 20, 20, testBoneColor)

	assertPixel(t, img, 80, 80, testBoneColor)
	assertPixel(t, img, 20, 20, testBoneColor)
	assertPixel(t, img, 50, 50, testBoneColor) // 对角线中点
	assertPixel(t, img, 15, 15, testEmptyPixel)
	assertPixel(t, img, 85, 85, testEmptyPixel)
}

func TestDrawSkeletonNoVisiblePerson(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	// 所有关键点可见度 0.2 < 0.3，不应绘制任何像素
	var p Person
	p.Box = [4]float32{10, 10, 90, 90}
	for i := range p.Keypoints {
		p.Keypoints[i] = [3]float32{float32(10 + i*4), 50, 0.2}
	}
	DrawSkeleton(img, []Person{p})

	if n := countDrawn(img); n != 0 {
		t.Errorf("drawn pixels = %d, want 0 (all keypoints invisible)", n)
	}

	DrawSkeleton(img, nil) // 空切片同样不应绘制
	if n := countDrawn(img); n != 0 {
		t.Errorf("drawn pixels after nil slice = %d, want 0", n)
	}
}

func TestDrawSkeletonVisiblePerson(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))

	var p Person
	p.Keypoints[0] = [3]float32{20, 20, 1.0} // nose（可见）
	p.Keypoints[1] = [3]float32{60, 20, 1.0} // left_eye（可见）
	p.Keypoints[2] = [3]float32{80, 80, 0.1} // right_eye（不可见）

	DrawSkeleton(img, []Person{p})

	if n := countDrawn(img); n == 0 {
		t.Fatal("visible person drew no pixels")
	}
	// 关节点为红色（在线之后绘制，覆盖线色）
	assertPixel(t, img, 20, 20, testJointColor)
	assertPixel(t, img, 60, 20, testJointColor)
	// nose→left_eye 连线中段为绿色
	assertPixel(t, img, 40, 20, testBoneColor)
	// 可见度不足的关键点既不画点也不参与连线
	assertPixel(t, img, 80, 80, testEmptyPixel)
}

func TestCOCO17Length(t *testing.T) {
	if len(COCO17) != 17 {
		t.Errorf("len(COCO17) = %d, want 17", len(COCO17))
	}
}

func TestSkeletonEdges(t *testing.T) {
	if len(SkeletonEdges) != 19 {
		t.Fatalf("len(SkeletonEdges) = %d, want 19", len(SkeletonEdges))
	}
	// 每条边的两个端点索引必须落在 COCO17 合法范围内
	for i, e := range SkeletonEdges {
		if e[0] < 0 || e[0] >= len(COCO17) || e[1] < 0 || e[1] >= len(COCO17) {
			t.Errorf("SkeletonEdges[%d] = %v, index out of range [0,%d)", i, e, len(COCO17))
		}
	}
}
