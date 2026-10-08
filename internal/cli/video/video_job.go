package video

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

import (
	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// openRouterJobInfo is saved to disk so the user can resume a timed-out video job.
type openRouterJobInfo struct {
	JobID      string `json:"job_id"`
	PollingURL string `json:"polling_url"`
	Model      string `json:"model"`
	Prompt     string `json:"prompt"`
	CreatedAt  int64  `json:"created_at"`
}

func jobFilePath(jobID string) string {
	return filepath.Join(options.Shared.OutputDir, fmt.Sprintf("video_job_%s.json", jobID))
}

func saveJobInfo(info *openRouterJobInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(jobFilePath(info.JobID), data, 0644)
}

func loadJobInfo(jobID string) (*openRouterJobInfo, error) {
	data, err := os.ReadFile(jobFilePath(jobID))
	if err != nil {
		return nil, fmt.Errorf("job file %s not found (was the job submitted with this output directory?): %w", jobFilePath(jobID), err)
	}
	var info openRouterJobInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("failed to parse job file: %w", err)
	}
	return &info, nil
}

// removeJobInfo deletes the persisted job file once its video has been fully
// downloaded. The file only exists to resume a job that has not been fetched
// yet, so it is redundant after a successful download (and the platform-side
// job expires after a while anyway). A missing file is not an error.
func removeJobInfo(jobID string) {
	path := jobFilePath(jobID)
	if err := os.Remove(path); err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove job file %s: %v\n", path, err)
		}
		return
	}
	fmt.Printf("Job file removed: %s\n", path)
}
