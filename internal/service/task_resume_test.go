package service

import (
	"strings"
	"testing"
)

func TestTaskResume(t *testing.T) {
	if got := TaskResume(""); got != "" {
		t.Fatalf("TaskResume(\"\") = %q, want empty", got)
	}
	if got := TaskResume("task_1"); !strings.Contains(got, "aigc-cli task task_1") {
		t.Fatalf("TaskResume() = %q", got)
	}
	if got := JobResume("job_1"); !strings.Contains(got, "aigc-cli video --job-id job_1") {
		t.Fatalf("JobResume() = %q", got)
	}
}
