package depth

import "testing"

func TestColorizeEndpoints(t *testing.T) {
	// 灰度 0（远）→ 冷色（蓝 > 红）
	r, g, b := colorize(0)
	if b <= r {
		t.Errorf("colorize(0) = (%d,%d,%d), want cool (B > R)", r, g, b)
	}
	// 灰度 255（近）→ 暖色（红 > 蓝）
	r, g, b = colorize(255)
	if r <= b {
		t.Errorf("colorize(255) = (%d,%d,%d), want warm (R > B)", r, g, b)
	}
}

func TestColorizeMonotonic(t *testing.T) {
	// 灰度单调递增时颜色连续变化，无突变
	prev := [3]int{}
	for i := 0; i <= 255; i++ {
		r, g, b := colorize(uint8(i))
		cur := [3]int{int(r), int(g), int(b)}
		if i > 0 {
			for c := 0; c < 3; c++ {
				diff := cur[c] - prev[c]
				if diff < -30 || diff > 30 {
					t.Errorf("colorize jump at %d: %v → %v", i, prev, cur)
				}
			}
		}
		prev = cur
	}
}

func TestDepthToColorCrop(t *testing.T) {
	// 画布 2x2，data 行优先：[0,0]=1.0, [1,0]=1.0, [0,1]=3.0(近), [1,1]=1.0
	data := []float32{1.0, 1.0, 3.0, 1.0}
	crop := Crop{X: 0, Y: 0, Width: 2, Height: 2}
	img := DepthToColorCrop(data, 2, crop, 2, 2)
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 2 {
		t.Fatalf("color crop size = %v, want 2x2", img.Bounds())
	}
	// 近处像素 (0,1)=3.0 应为暖色
	near := img.RGBAAt(0, 1)
	far := img.RGBAAt(0, 0)
	if near.R <= near.B {
		t.Errorf("near pixel = %+v, want warm (R>B)", near)
	}
	if far.B <= far.R {
		t.Errorf("far pixel = %+v, want cool (B>R)", far)
	}
}
