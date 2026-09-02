package gif

import "testing"

func TestBuildFilter(t *testing.T) {
	tests := []struct {
		name  string
		width int
		crop  CropMargins
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
		{
			name:  "crop all sides",
			width: 160,
			crop:  CropMargins{Top: 40, Right: 40, Bottom: 40, Left: 40},
			want:  "fps=6,crop=iw-80:ih-80:40:40,scale=160:-2:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none",
		},
		{
			name:  "crop top and bottom only",
			width: 160,
			crop:  CropMargins{Top: 64, Bottom: 64},
			want:  "fps=6,crop=iw-0:ih-128:0:64,scale=160:-2:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none",
		},
		{
			name:  "crop bottom edge only",
			width: 160,
			crop:  CropMargins{Bottom: 40},
			want:  "fps=6,crop=iw-0:ih-40:0:0,scale=160:-2:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none",
		},
		{
			name:  "asymmetric crop",
			width: 160,
			crop:  CropMargins{Top: 40, Right: 30, Bottom: 20, Left: 10},
			want:  "fps=6,crop=iw-40:ih-60:10:40,scale=160:-2:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none",
		},
		{
			name:  "crop keep original size",
			width: 0,
			crop:  CropMargins{Top: 40, Right: 40, Bottom: 40, Left: 40},
			want:  "fps=6,crop=iw-80:ih-80:40:40,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=none",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildFilter(tt.width, tt.crop); got != tt.want {
				t.Errorf("buildFilter(%d, %+v) = %q, want %q", tt.width, tt.crop, got, tt.want)
			}
		})
	}
}

func TestBuildCropFilter(t *testing.T) {
	tests := []struct {
		name string
		crop CropMargins
		want string
	}{
		{name: "all sides", crop: CropMargins{Top: 40, Right: 40, Bottom: 40, Left: 40}, want: "crop=iw-80:ih-80:40:40"},
		{name: "top/bottom only", crop: CropMargins{Top: 64, Bottom: 64}, want: "crop=iw-0:ih-128:0:64"},
		{name: "asymmetric", crop: CropMargins{Top: 40, Right: 30, Bottom: 20, Left: 10}, want: "crop=iw-40:ih-60:10:40"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildCropFilter(tt.crop); got != tt.want {
				t.Errorf("buildCropFilter(%+v) = %q, want %q", tt.crop, got, tt.want)
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
