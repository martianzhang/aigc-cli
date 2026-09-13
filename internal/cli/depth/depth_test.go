package depth

import "testing"

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"single", "abc", []string{"abc"}},
		{"multiple", "a b c", []string{"a", "b", "c"}},
		{"flags", "-crf 23 -preset medium", []string{"-crf", "23", "-preset", "medium"}},
		{"quoted value", `-x "-a b" y`, []string{"-x", "-a b", "y"}},
		{"extra spaces", "  a   b  ", []string{"a", "b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitArgs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("splitArgs(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("splitArgs(%q) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}
