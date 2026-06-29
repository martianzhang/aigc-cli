package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- previewLatestFiles ---

func TestPreviewLatestFiles_emptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	oldOutput := shared.OutputDir
	shared.OutputDir = tmpDir
	defer func() { shared.OutputDir = oldOutput }()

	files := previewLatestFiles("image_")
	if len(files) != 0 {
		t.Errorf("previewLatestFiles() = %v, want empty", files)
	}
}

func TestPreviewLatestFiles_nonexistentDir(t *testing.T) {
	oldOutput := shared.OutputDir
	shared.OutputDir = "/tmp/nonexistent_dir_xyz"
	defer func() { shared.OutputDir = oldOutput }()

	files := previewLatestFiles("image_")
	if files != nil {
		t.Errorf("previewLatestFiles() should be nil for nonexistent dir, got %v", files)
	}
}

func TestPreviewLatestFiles_findsLatest(t *testing.T) {
	tmpDir := t.TempDir()
	oldOutput := shared.OutputDir
	shared.OutputDir = tmpDir
	defer func() { shared.OutputDir = oldOutput }()

	// Create some files with different timestamps
	oldFile := filepath.Join(tmpDir, "image_old.png")
	os.WriteFile(oldFile, []byte("old"), 0644)
	os.Chtimes(oldFile, time.Now(), time.Now().Add(-1*time.Hour))

	newFile := filepath.Join(tmpDir, "image_new.png")
	os.WriteFile(newFile, []byte("new"), 0644)
	os.Chtimes(newFile, time.Now(), time.Now())

	files := previewLatestFiles("image_")
	if len(files) == 0 {
		t.Fatal("previewLatestFiles() returned no files")
	}
	if !strings.HasSuffix(files[0], "image_new.png") {
		t.Errorf("previewLatestFiles()[0] = %q, want newest file", files[0])
	}
}

func TestPreviewLatestFiltersByPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	oldOutput := shared.OutputDir
	shared.OutputDir = tmpDir
	defer func() { shared.OutputDir = oldOutput }()

	os.WriteFile(filepath.Join(tmpDir, "image_001.png"), []byte("img"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "video_001.mp4"), []byte("vid"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "readme.md"), []byte("doc"), 0644)

	files := previewLatestFiles("image_")
	if len(files) != 1 {
		t.Errorf("previewLatestFiles('image_') = %v, want 1 file", files)
	}

	files = previewLatestFiles("video_")
	if len(files) != 1 {
		t.Errorf("previewLatestFiles('video_') = %v, want 1 file", files)
	}
}

func TestPreviewLatestLimitsToThree(t *testing.T) {
	tmpDir := t.TempDir()
	oldOutput := shared.OutputDir
	shared.OutputDir = tmpDir
	defer func() { shared.OutputDir = oldOutput }()

	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(tmpDir, "image_xxx.png"), []byte("img"), 0644)
	}

	files := previewLatestFiles("image_")
	if len(files) > 3 {
		t.Errorf("previewLatestFiles() returned %d files, want max 3", len(files))
	}
}

func TestPreviewLatestSkipsDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	oldOutput := shared.OutputDir
	shared.OutputDir = tmpDir
	defer func() { shared.OutputDir = oldOutput }()

	os.MkdirAll(filepath.Join(tmpDir, "image_subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "image_real.png"), []byte("img"), 0644)

	files := previewLatestFiles("image_")
	if len(files) != 1 {
		t.Errorf("previewLatestFiles() = %v, want 1 (skipping dirs)", files)
	}
}
