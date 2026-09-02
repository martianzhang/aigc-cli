package gif

import "testing"

func TestParseCropMargin(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    CropMargins
		wantErr bool
	}{
		{name: "empty means no crop", in: "", want: CropMargins{}},
		{name: "whitespace means no crop", in: "  ", want: CropMargins{}},
		{name: "one value all sides", in: "40", want: CropMargins{Top: 40, Right: 40, Bottom: 40, Left: 40}},
		{name: "two values top/bottom, left/right", in: "40,0", want: CropMargins{Top: 40, Right: 0, Bottom: 40, Left: 0}},
		{name: "three values top, sides, bottom", in: "40,0,60", want: CropMargins{Top: 40, Right: 0, Bottom: 60, Left: 0}},
		{name: "four values top,right,bottom,left", in: "40,30,20,10", want: CropMargins{Top: 40, Right: 30, Bottom: 20, Left: 10}},
		{name: "space separated", in: "40 0", want: CropMargins{Top: 40, Right: 0, Bottom: 40, Left: 0}},
		{name: "mixed separators with spaces", in: " 40 , 30 , 20 , 10 ", want: CropMargins{Top: 40, Right: 30, Bottom: 20, Left: 10}},
		{name: "too many values", in: "1,2,3,4,5", wantErr: true},
		{name: "negative value", in: "-5", wantErr: true},
		{name: "non-numeric", in: "abc", wantErr: true},
		{name: "partial non-numeric", in: "40,abc", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCropMargin(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseCropMargin(%q) expected error, got %+v", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCropMargin(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("ParseCropMargin(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestCropMarginsZero(t *testing.T) {
	if !(CropMargins{}).Zero() {
		t.Error("zero value should be Zero")
	}
	if (CropMargins{Top: 1}).Zero() {
		t.Error("non-zero crop should not be Zero")
	}
}

func TestCropMarginsString(t *testing.T) {
	tests := []struct {
		in   CropMargins
		want string
	}{
		{in: CropMargins{}, want: "0"},
		{in: CropMargins{Top: 40, Right: 40, Bottom: 40, Left: 40}, want: "40"},
		{in: CropMargins{Top: 40, Right: 0, Bottom: 40, Left: 0}, want: "40,0"},
		{in: CropMargins{Top: 40, Right: 30, Bottom: 20, Left: 10}, want: "40,30,20,10"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("(%+v).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}
