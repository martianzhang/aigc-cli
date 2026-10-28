package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestDownloadLastFrames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("frame-bytes"))
	}))
	defer srv.Close()

	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rewriteTransport{target: srv.URL}}
	defer func() { http.DefaultClient = oldClient }()

	dir := t.TempDir()
	// A blank URL is skipped; indices are preserved in the multi-frame naming.
	urls := []string{
		"https://cdn.example.com/a.png",
		"",
		"https://cdn.example.com/b.png",
	}
	saved, err := DownloadLastFrames(urls, dir, "task_1")
	if err != nil {
		t.Fatalf("DownloadLastFrames() error = %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("saved = %v, want 2 files", saved)
	}
	want := []string{
		filepath.Join(dir, "task_1_lastframe_0.png"),
		filepath.Join(dir, "task_1_lastframe_2.png"),
	}
	for i, f := range want {
		if saved[i] != f {
			t.Errorf("saved[%d] = %q, want %q", i, saved[i], f)
		}
		data, rerr := os.ReadFile(f)
		if rerr != nil || string(data) != "frame-bytes" {
			t.Errorf("saved file %q content = %q (err %v), want frame-bytes", f, data, rerr)
		}
	}
}

func TestDownloadLastFramesSingleName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: rewriteTransport{target: srv.URL}}
	defer func() { http.DefaultClient = oldClient }()

	dir := t.TempDir()
	saved, err := DownloadLastFrames([]string{"https://cdn.example.com/only.png"}, dir, "task_2")
	if err != nil {
		t.Fatalf("DownloadLastFrames() error = %v", err)
	}
	if len(saved) != 1 || saved[0] != filepath.Join(dir, "task_2_lastframe.png") {
		t.Errorf("saved = %v, want [task_2_lastframe.png]", saved)
	}
}

func TestDownloadLastFramesAllEmpty(t *testing.T) {
	saved, err := DownloadLastFrames([]string{"", "  "}, t.TempDir(), "task_3")
	if err != nil {
		t.Fatalf("DownloadLastFrames() error = %v", err)
	}
	if len(saved) != 0 {
		t.Errorf("saved = %v, want none", saved)
	}
}

func TestDownloadVideosAllFailedReturnsError(t *testing.T) {
	videos := []types.VideoResult{{URL: []string{"not-a-valid-source"}}}
	saved, err := DownloadVideos(videos, t.TempDir(), "task_1")
	if err == nil {
		t.Fatal("DownloadVideos() error = nil, want failure when nothing was saved")
	}
	if len(saved) != 0 {
		t.Errorf("saved = %v, want none", saved)
	}
}
