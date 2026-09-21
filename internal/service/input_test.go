package service

import (
	"os"
	"strings"
	"testing"
)

func TestExtractExt(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://example.com/video.mp4", ".mp4"},
		{"https://example.com/video", ".mp4"},
		{"https://example.com/photo.jpg", ".jpg"},
		{"https://example.com/video.mp4?token=abc", ".mp4"},
	}
	for _, tc := range tests {
		if got := ExtractExt(tc.in); got != tc.want {
			t.Errorf("ExtractExt(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsFile(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		tmp, _ := os.CreateTemp("", "testfile")
		tmp.Close()
		defer os.Remove(tmp.Name())
		if !IsFile(tmp.Name()) {
			t.Errorf("IsFile(%q) should be true", tmp.Name())
		}
	})

	t.Run("not exists", func(t *testing.T) {
		if IsFile("/tmp/nonexistent_file_xyz") {
			t.Error("IsFile() should be false for nonexistent file")
		}
	})

	t.Run("directory", func(t *testing.T) {
		dir, _ := os.MkdirTemp("", "testdir")
		defer os.Remove(dir)
		if IsFile(dir) {
			t.Error("IsFile() should be false for directory")
		}
	})
}

func TestReadInput(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		got, err := ReadInput("hello world")
		if err != nil {
			t.Fatalf("ReadInput() error = %v", err)
		}
		if string(got) != "hello world" {
			t.Errorf("ReadInput() = %q, want %q", string(got), "hello world")
		}
	})

	t.Run("file", func(t *testing.T) {
		tmp, _ := os.CreateTemp("", "testinput")
		tmp.WriteString("file content")
		tmp.Close()
		defer os.Remove(tmp.Name())
		got, err := ReadInput(tmp.Name())
		if err != nil {
			t.Fatalf("ReadInput() error = %v", err)
		}
		if string(got) != "file content" {
			t.Errorf("ReadInput() = %q, want %q", string(got), "file content")
		}
	})
}

func TestReadJSONInput(t *testing.T) {
	t.Run("inline object", func(t *testing.T) {
		const raw = `{"prompt":"a cat"}`
		got, err := ReadJSONInput(raw)
		if err != nil {
			t.Fatalf("ReadJSONInput() error = %v", err)
		}
		if string(got) != raw {
			t.Errorf("ReadJSONInput() = %q, want %q", string(got), raw)
		}
	})

	t.Run("inline array with leading space", func(t *testing.T) {
		got, err := ReadJSONInput("  [1,2]")
		if err != nil {
			t.Fatalf("ReadJSONInput() error = %v", err)
		}
		if string(got) != "  [1,2]" {
			t.Errorf("ReadJSONInput() = %q, want %q", string(got), "  [1,2]")
		}
	})

	t.Run("existing file", func(t *testing.T) {
		tmp, _ := os.CreateTemp("", "testjson")
		tmp.WriteString(`{"prompt":"x"}`)
		tmp.Close()
		defer os.Remove(tmp.Name())
		got, err := ReadJSONInput(tmp.Name())
		if err != nil {
			t.Fatalf("ReadJSONInput() error = %v", err)
		}
		if string(got) != `{"prompt":"x"}` {
			t.Errorf("ReadJSONInput() = %q", string(got))
		}
	})

	t.Run("missing path reports file not found", func(t *testing.T) {
		_, err := ReadJSONInput("downloads/does-not-exist.json")
		if err == nil {
			t.Fatal("ReadJSONInput() error = nil, want file not found")
		}
		if !strings.Contains(err.Error(), "file not found") {
			t.Errorf("ReadJSONInput() error = %v, want it to mention 'file not found'", err)
		}
	})

	t.Run("inline jsonc with leading comment", func(t *testing.T) {
		got, err := ReadJSONInput("// note\n{\"prompt\":\"x\"}")
		if err != nil {
			t.Fatalf("ReadJSONInput() error = %v", err)
		}
		if strings.Contains(string(got), "note") {
			t.Errorf("ReadJSONInput() = %q, want the comment stripped", string(got))
		}
		if !strings.Contains(string(got), `"prompt":"x"`) {
			t.Errorf("ReadJSONInput() = %q, want the JSON preserved", string(got))
		}
	})
}

func TestIsImageInput(t *testing.T) {
	imageCases := []string{"a.png", "b.jpg", "c.jpeg", "d.webp", "e.bmp", "f.gif", "g.avif", "h.heic", "i.jxl", "IMG_001.PNG"}
	videoCases := []string{"a.mp4", "b.mov", "c.mkv", "d.avi", "e.webm", "noext", "a.tar.gz"}
	for _, c := range imageCases {
		if !IsImageInput(c) {
			t.Errorf("IsImageInput(%q) = false, want true", c)
		}
	}
	for _, c := range videoCases {
		if IsImageInput(c) {
			t.Errorf("IsImageInput(%q) = true, want false", c)
		}
	}
}
