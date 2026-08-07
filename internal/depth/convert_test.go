package depth

import "testing"

func TestParseTimeToSeconds(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{"empty", "", 0},
		{"seconds", "3", 3},
		{"seconds_float", "2.5", 2.5},
		{"mmss", "01:30", 90},
		{"hhmmss", "01:02:03", 3723},
		{"zero", "0", 0},
		{"leading_zeros", "00:05", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTimeToSeconds(tt.in)
			if err != nil {
				t.Fatalf("parseTimeToSeconds(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseTimeToSeconds(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTimeToSecondsInvalid(t *testing.T) {
	for _, in := range []string{"abc", "1:2:3:4", "-5", "1::2", ":10"} {
		t.Run(in, func(t *testing.T) {
			if _, err := parseTimeToSeconds(in); err == nil {
				t.Errorf("parseTimeToSeconds(%q) should error", in)
			}
		})
	}
}

func TestPercentileRange(t *testing.T) {
	// 100 pixels: values 0..99. 1st percentile ≈ 1, 99th ≈ 98.
	data := make([]uint8, 100)
	for i := range data {
		data[i] = uint8(i)
	}
	lo, hi := percentileRange(data, 1.0, 99.0)
	if lo != 1 || hi != 98 {
		t.Errorf("percentileRange = (%v, %v), want (1, 98)", lo, hi)
	}
}

func TestPercentileRangeUniform(t *testing.T) {
	data := make([]uint8, 50)
	for i := range data {
		data[i] = 128
	}
	lo, hi := percentileRange(data, 1.0, 99.0)
	if lo != 128 || hi != 128 {
		t.Errorf("uniform percentileRange = (%v, %v), want (128, 128)", lo, hi)
	}
}

func TestPercentileRangeEmpty(t *testing.T) {
	lo, hi := percentileRange(nil, 1.0, 99.0)
	if lo != 0 || hi != 255 {
		t.Errorf("empty percentileRange = (%v, %v), want (0, 255)", lo, hi)
	}
}

func TestPercentileRangeExtremes(t *testing.T) {
	data := []uint8{100, 100, 100, 100, 0, 255, 100, 100, 100, 100}
	lo, hi := percentileRange(data, 10.0, 90.0)
	if lo != 100 || hi != 100 {
		t.Errorf("percentileRange = (%v, %v), want (100, 100)", lo, hi)
	}
}

func TestParseEncodeArgs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"simple", "-crf 28", []string{"-crf", "28"}},
		{"quoted", `-vf "scale=100:100"`, []string{"-vf", "scale=100:100"}},
		{"mixed", `-crf 28 -preset slow -vf "scale=100:100"`, []string{"-crf", "28", "-preset", "slow", "-vf", "scale=100:100"}},
		{"spaces", "  -crf   28  ", []string{"-crf", "28"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEncodeArgs(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("parseEncodeArgs(%q) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseEncodeArgs(%q) = %v, want %v", tt.in, got, tt.want)
				}
			}
		})
	}
}
