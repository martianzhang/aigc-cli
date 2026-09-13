package cmd

import "testing"

func TestCmdFormatBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1023, "1023B"},
		{1024, "1.0KB"},
		{1536, "1.5KB"},
		{1024 * 1024, "1.0MB"},
		{1024 * 1024 * 3 / 2, "1.5MB"},
	}
	for _, tc := range tests {
		if got := formatBytes(tc.in); got != tc.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCmdFormatParams(t *testing.T) {
	tests := []struct {
		format  string
		quality int
		want    string
	}{
		{"", 0, ""},
		{"jpg", 0, " [jpg]"},
		{"jpg", 85, " [jpg q85]"},
		{"webp", -1, " [webp]"},
	}
	for _, tc := range tests {
		if got := formatParams(tc.format, tc.quality); got != tc.want {
			t.Errorf("formatParams(%q,%d) = %q, want %q", tc.format, tc.quality, got, tc.want)
		}
	}
}

func TestCmdParsePageRange(t *testing.T) {
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

func TestCmdSplitArgs(t *testing.T) {
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

func TestCmdMaskBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"host only", "https://api.apimart.ai", "api.apimart.ai"},
		{"host with port", "https://api.apimart.ai:8080/v1", "api.apimart.ai:8080"},
		{"invalid passthrough", "://bad", "://bad"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskBaseURL(tc.in); got != tc.want {
				t.Errorf("maskBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
