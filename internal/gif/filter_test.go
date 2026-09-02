package gif

import "testing"

func TestBuildFilter(t *testing.T) {
	tests := []struct {
		name  string
		width int
		want  string
	}{
		{
			name:  "keep original size",
			width: 0,
			want:  "fps=6,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none",
		},
		{
			name:  "specified width",
			width: 160,
			want:  "fps=6,scale=160:-2:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none",
		},
		{
			name:  "another width",
			width: 320,
			want:  "fps=6,scale=320:-2:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildFilter(tt.width); got != tt.want {
				t.Errorf("buildFilter(%d) = %q, want %q", tt.width, got, tt.want)
			}
		})
	}
}

func TestDefaultOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		width int
		want  string
	}{
		{name: "with width", input: "/tmp/video.mp4", width: 160, want: "video_160px.gif"},
		{name: "original size", input: "/tmp/video.mp4", width: 0, want: "video.gif"},
		{name: "preserve stem", input: "dir/pushup.MOV", width: 320, want: "pushup_320px.gif"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultOutput(tt.input, tt.width); got != tt.want {
				t.Errorf("defaultOutput(%q, %d) = %q, want %q", tt.input, tt.width, got, tt.want)
			}
		})
	}
}
