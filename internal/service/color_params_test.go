package service

import (
	"image/color"
	"testing"
)

func TestParseHexColor(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    color.RGBA
		wantErr bool
	}{
		{"with hash", "#ff0000", color.RGBA{R: 255, G: 0, B: 0, A: 255}, false},
		{"without hash", "ff0000", color.RGBA{R: 255, G: 0, B: 0, A: 255}, false},
		{"mixed", "#A1b2C3", color.RGBA{R: 0xA1, G: 0xB2, B: 0xC3, A: 255}, false},
		{"white", "#ffffff", color.RGBA{R: 255, G: 255, B: 255, A: 255}, false},
		{"too short", "#f00", color.RGBA{}, true},
		{"not hex", "#zzzzzz", color.RGBA{}, true},
		{"empty", "", color.RGBA{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseHexColor(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseHexColor(%q) expected error", tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseHexColor(%q) unexpected error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseHexColor(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseOffset(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		dx, dy, err := ParseOffset("10,-20")
		if err != nil || dx != 10 || dy != -20 {
			t.Fatalf("got (%d,%d,%v), want (10,-20,nil)", dx, dy, err)
		}
	})

	for _, bad := range []string{"10", "10,20,30", "a,b"} {
		t.Run("invalid "+bad, func(t *testing.T) {
			if _, _, err := ParseOffset(bad); err == nil {
				t.Fatalf("ParseOffset(%q) expected error", bad)
			}
		})
	}
}
