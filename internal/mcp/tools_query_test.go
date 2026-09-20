package mcp

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestSafeFileToken(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"../../evil", "evil"},
		{"../../../../evil", "evil"},
		{"task_abc123", "task_abc123"},
		{"/etc/passwd", "passwd"},
		{"a/b", "b"},
		{`a\..\b`, "a___b"},
		{"..", "_"},
		{".", "task"},
		{"", "task"},
	}
	for _, c := range cases {
		if got := safeFileToken(c.in); got != c.want {
			t.Errorf("safeFileToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHandleMCPGetAPIMartTask_traversalIDContained(t *testing.T) {
	dir := t.TempDir()
	uri := tinyPNGDataURI(t)
	mock := &mockAPIClient{
		getTaskFn: func(taskID string) (*types.TaskData, error) {
			return &types.TaskData{
				ID: taskID, Status: "completed", Progress: 100,
				Result: &types.TaskResult{
					Images: []types.ImageResult{{URL: []string{uri}}},
				},
			}, nil
		},
	}

	res, err := handleMCPGetAPIMartTask(mock, "../../../../evil", dir)
	if err != nil {
		t.Fatalf("handleMCPGetAPIMartTask: %v", err)
	}
	text := resultText(t, res)
	want := filepath.Join(dir, "image_evil_0_0.png")
	if !strings.Contains(text, want) {
		t.Errorf("result text = %q, want path %q", text, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("download not contained in output dir: %v", err)
	}
}

func TestHandleMCPGetOpenRouterJob_traversalIDContained(t *testing.T) {
	dir := t.TempDir()
	uri := tinyPNGDataURI(t)
	mock := &mockAPIClient{
		openRouterVideoGetFn: func(jobID string) (*types.OpenRouterVideoStatusResponse, error) {
			return &types.OpenRouterVideoStatusResponse{
				ID: jobID, Status: "completed",
				UnsignedURLs: []string{uri},
			}, nil
		},
	}

	res, err := handleMCPGetOpenRouterJob(mock, "../../../../evil", dir, "test-key")
	if err != nil {
		t.Fatalf("handleMCPGetOpenRouterJob: %v", err)
	}
	text := resultText(t, res)
	want := filepath.Join(dir, "video_evil_0.png")
	if !strings.Contains(text, want) {
		t.Errorf("result text = %q, want path %q", text, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("download not contained in output dir: %v", err)
	}
}

func tinyPNGDataURI(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}
