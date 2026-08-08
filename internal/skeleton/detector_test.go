package skeleton

import "testing"

func TestPostprocessLayout(t *testing.T) {
	// 构造完整输出 [56, 8400]：第 0 个 anchor conf 高且有有效 box
	out := make([]float32, OutChannels*NumAnchors)
	// anchor0: box(xywh=100,100,50,50), conf=0.9, nose kpt
	out[0*NumAnchors+0] = 100 // cx
	out[1*NumAnchors+0] = 100 // cy
	out[2*NumAnchors+0] = 50  // w
	out[3*NumAnchors+0] = 50  // h
	out[4*NumAnchors+0] = 0.9 // conf
	out[5*NumAnchors+0] = 100 // kpt0 x (nose)
	out[6*NumAnchors+0] = 100 // kpt0 y
	out[7*NumAnchors+0] = 1.0 // kpt0 vis

	people := postprocess(out, 1000, 1000, 0.5, 0, 0, 0.5)
	if len(people) != 1 {
		t.Fatalf("people = %d, want 1", len(people))
	}
	p := people[0]
	// box 反算回原图 (scale=0.5, 无 pad): (cx-pad)/scale
	if p.Box[0] != 150 || p.Box[1] != 150 || p.Box[2] != 250 || p.Box[3] != 250 {
		t.Errorf("box = %v, want [150 150 250 250]", p.Box)
	}
	if p.Keypoints[0][0] != 200 || p.Keypoints[0][1] != 200 {
		t.Errorf("nose = %v, want [200 200]", p.Keypoints[0])
	}
}

func TestIOU(t *testing.T) {
	a := [4]float32{0, 0, 10, 10}
	b := [4]float32{5, 5, 15, 15}
	if v := iou(a, b); v != 0.14285715 {
		t.Errorf("iou = %v, want ~0.143", v)
	}
	// 不相交
	c := [4]float32{20, 20, 30, 30}
	if v := iou(a, c); v != 0 {
		t.Errorf("iou disjoint = %v, want 0", v)
	}
}
