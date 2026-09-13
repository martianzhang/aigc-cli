package ocr

import "testing"

func TestParsePageRange(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "5", []string{"5"}},
		{"mixed ranges", "1-3,5,7-9", []string{"1-3", "5", "7-9"}},
		{"spaces trimmed", " 1 , 2 ", []string{"1", "2"}},
		{"blank tokens dropped", "1,,3", []string{"1", "3"}},
		{"only separators", ",", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePageRange(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("parsePageRange(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parsePageRange(%q) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}
