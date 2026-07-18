package cmd

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// ============================================================================
// MJ flag helpers
// ============================================================================

// ============================================================================
// buildMJCurl
// ============================================================================

func TestBuildMJCurl_imagine(t *testing.T) {
	shared.APIKey = "test-key-123"
	shared.APIBase = "https://api.apimart.ai"

	req := &types.MJImagineRequest{
		Prompt: "a cute cat",
		Size:   "16:9",
	}
	curl := buildMJCurl("imagine", req)

	if curl == "" {
		t.Fatal("buildMJCurl() returned empty string")
	}
	if !strings.Contains(curl, "test-key-123") {
		t.Error("curl should contain API key")
	}
	if !strings.Contains(curl, "/midjourney/generations/imagine") {
		t.Error("curl should contain correct MJ path")
	}
	if !strings.Contains(curl, "a cute cat") {
		t.Error("curl should contain prompt")
	}
	if !strings.Contains(curl, "16:9") {
		t.Error("curl should contain size")
	}
}

func TestBuildMJCurl_upscale(t *testing.T) {
	shared.APIKey = "test-key"
	shared.APIBase = "https://api.apimart.ai"

	idx := 1
	req := &types.MJTaskActionRequest{
		TaskID: "task_abc123",
		Index:  &idx,
	}
	curl := buildMJCurl("upscale", req)

	if !strings.Contains(curl, "/midjourney/generations/upscale") {
		t.Error("curl should contain upscale path")
	}
	if !strings.Contains(curl, "task_abc123") {
		t.Error("curl should contain task_id")
	}
}

func TestBuildMJCurl_blend(t *testing.T) {
	shared.APIKey = "test-key"
	shared.APIBase = "https://api.apimart.ai"

	req := &types.MJBlendRequest{
		ImageURLs:  []string{"a.png", "b.png"},
		Dimensions: "SQUARE",
	}
	curl := buildMJCurl("blend", req)

	if !strings.Contains(curl, "/midjourney/generations/blend") {
		t.Error("curl should contain blend path")
	}
	if !strings.Contains(curl, "SQUARE") {
		t.Error("curl should contain dimensions")
	}
}

func TestBuildMJCurl_customBaseURL(t *testing.T) {
	shared.APIKey = "test-key"
	shared.APIBase = "https://custom-relay.com/v1"

	req := &types.MJImagineRequest{Prompt: "test"}
	curl := buildMJCurl("imagine", req)

	if !strings.Contains(curl, "custom-relay.com/v1/midjourney/generations/imagine") {
		t.Errorf("curl should use custom base URL, got: %s", curl)
	}
}

// ============================================================================
// buildMJTaskActionReq
// ============================================================================

func TestBuildMJTaskActionReq_requiresTaskID(t *testing.T) {
	// Reset globals
	mjTaskID = ""
	mjIndex = 0
	mjCustomID = ""

	_, err := buildMJTaskActionReq()
	if err == nil || !strings.Contains(err.Error(), "--task-id") {
		t.Errorf("expected '--task-id is required' error, got: %v", err)
	}
}

func TestBuildMJTaskActionReq_withIndex(t *testing.T) {
	mjTaskID = "task_abc"
	mjIndex = 2
	mjCustomID = ""
	mjSpeed = "fast"

	req, err := buildMJTaskActionReq()
	if err != nil {
		t.Fatalf("buildMJTaskActionReq() error = %v", err)
	}

	if req.TaskID != "task_abc" {
		t.Errorf("TaskID = %q, want task_abc", req.TaskID)
	}
	if req.Index == nil || *req.Index != 2 {
		t.Errorf("Index = %v, want 2", req.Index)
	}
	if req.Speed != "fast" {
		t.Errorf("Speed = %q, want fast", req.Speed)
	}
}

func TestBuildMJTaskActionReq_withCustomID(t *testing.T) {
	mjTaskID = "task_abc"
	mjIndex = 0
	mjCustomID = "MJ::JOB::upsample::1::xyz"

	req, err := buildMJTaskActionReq()
	if err != nil {
		t.Fatalf("buildMJTaskActionReq() error = %v", err)
	}

	if req.CustomID != "MJ::JOB::upsample::1::xyz" {
		t.Errorf("CustomID = %q", req.CustomID)
	}
	// Index should be nil when not explicitly set to >0
	if req.Index != nil {
		t.Errorf("Index should be nil when not set to >0, got %v", *req.Index)
	}
}

// ============================================================================
// buildMJTaskActionReqFromJSON
// ============================================================================

func TestBuildMJTaskActionReqFromJSON(t *testing.T) {
	mjJSONInput = `{"task_id": "task_json", "index": 3, "speed": "turbo"}`

	req, err := buildMJTaskActionReqFromJSON()
	if err != nil {
		t.Fatalf("buildMJTaskActionReqFromJSON() error = %v", err)
	}

	if req.TaskID != "task_json" {
		t.Errorf("TaskID = %q, want task_json", req.TaskID)
	}
	if req.Index == nil || *req.Index != 3 {
		t.Errorf("Index = %v, want 3", req.Index)
	}
	if req.Speed != "turbo" {
		t.Errorf("Speed = %q, want turbo", req.Speed)
	}
}

func TestBuildMJTaskActionReqFromJSON_missingTaskID(t *testing.T) {
	mjJSONInput = `{"index": 1}`

	_, err := buildMJTaskActionReqFromJSON()
	if err == nil || !strings.Contains(err.Error(), "task_id") {
		t.Errorf("expected 'task_id is required' error, got: %v", err)
	}
}
