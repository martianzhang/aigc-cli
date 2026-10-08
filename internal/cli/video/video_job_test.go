package video

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// withJobDir points the shared output dir at a temp dir and restores the
// global job-related state when the test finishes.
func withJobDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldOutput := options.Shared.OutputDir
	options.Shared.OutputDir = dir
	t.Cleanup(func() {
		options.Shared.OutputDir = oldOutput
	})
	return dir
}

func TestJobInfoSaveLoad(t *testing.T) {
	dir := withJobDir(t)

	info := &openRouterJobInfo{
		JobID:      "gen-vid-test",
		PollingURL: "https://openrouter.ai/api/v1/videos/gen-vid-test",
		Model:      "x-ai/grok-imagine-video-1.5",
		Prompt:     "rainy night neon sign",
		CreatedAt:  123,
	}
	if err := saveJobInfo(info); err != nil {
		t.Fatalf("saveJobInfo: %v", err)
	}

	path := filepath.Join(dir, "video_job_gen-vid-test.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("job file not written: %v", err)
	}

	loaded, err := loadJobInfo(info.JobID)
	if err != nil {
		t.Fatalf("loadJobInfo: %v", err)
	}
	if loaded.JobID != info.JobID || loaded.PollingURL != info.PollingURL ||
		loaded.Model != info.Model || loaded.Prompt != info.Prompt || loaded.CreatedAt != info.CreatedAt {
		t.Fatalf("loaded job mismatch:\n got %+v\nwant %+v", loaded, info)
	}
}

func TestRemoveJobInfoDeletesAfterDownload(t *testing.T) {
	dir := withJobDir(t)
	path := filepath.Join(dir, "video_job_gen-vid-test.json")
	if err := saveJobInfo(&openRouterJobInfo{JobID: "gen-vid-test"}); err != nil {
		t.Fatalf("saveJobInfo: %v", err)
	}

	removeJobInfo("gen-vid-test")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("job file should be removed after a successful download, stat err = %v", err)
	}
}

func TestRemoveJobInfoMissingFileIsNoop(t *testing.T) {
	withJobDir(t)
	// Must not panic or report an error when there is nothing to remove.
	removeJobInfo("gen-vid-absent")
}

func TestLoadJobInfoMissingFileErrors(t *testing.T) {
	withJobDir(t)
	if _, err := loadJobInfo("gen-vid-absent"); err == nil {
		t.Fatal("loadJobInfo should error for a missing job file")
	}
}
