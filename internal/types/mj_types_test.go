package types

import (
	"encoding/json"
	"testing"
)

// ============================================================================
// MJImagineRequest JSON marshaling
// ============================================================================

func TestMJImagineRequest_marshal(t *testing.T) {
	v := 6
	req := MJImagineRequest{
		Prompt:    "a cute cat",
		Size:      "16:9",
		Version:   "6.1",
		Style:     "raw",
		Speed:     "fast",
		Seed:      &v,
		ImageURLs: []string{"https://example.com/img.png"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got map[string]any
	json.Unmarshal(data, &got)

	if got["prompt"] != "a cute cat" {
		t.Errorf("prompt = %v, want 'a cute cat'", got["prompt"])
	}
	if got["size"] != "16:9" {
		t.Errorf("size = %v, want '16:9'", got["size"])
	}
	if got["version"] != "6.1" {
		t.Errorf("version = %v, want '6.1'", got["version"])
	}
	if got["style"] != "raw" {
		t.Errorf("style = %v, want 'raw'", got["style"])
	}
	if got["speed"] != "fast" {
		t.Errorf("speed = %v, want 'fast'", got["speed"])
	}
	if got["seed"] != float64(6) {
		t.Errorf("seed = %v, want 6", got["seed"])
	}
}

func TestMJImagineRequest_marshalOmitEmpty(t *testing.T) {
	req := MJImagineRequest{
		Prompt: "hello",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got map[string]any
	json.Unmarshal(data, &got)

	// Only prompt should be present; everything else omitempty
	if len(got) != 1 && got["prompt"] != "hello" {
		t.Errorf("expected only prompt, got %v", got)
	}
}

func TestMJImagineRequest_unmarshal(t *testing.T) {
	src := `{
		"prompt": "a cute cat",
		"size": "16:9",
		"version": "6.1",
		"speed": "fast",
		"seed": 42,
		"image_urls": ["https://example.com/img.png"]
	}`

	var req MJImagineRequest
	if err := json.Unmarshal([]byte(src), &req); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if req.Prompt != "a cute cat" {
		t.Errorf("Prompt = %q, want %q", req.Prompt, "a cute cat")
	}
	if req.Size != "16:9" {
		t.Errorf("Size = %q, want %q", req.Size, "16:9")
	}
	if req.Seed == nil || *req.Seed != 42 {
		t.Errorf("Seed = %v, want 42", req.Seed)
	}
	if len(req.ImageURLs) != 1 || req.ImageURLs[0] != "https://example.com/img.png" {
		t.Errorf("ImageURLs = %v, want [https://example.com/img.png]", req.ImageURLs)
	}
}

// ============================================================================
// MJBlendRequest JSON marshaling
// ============================================================================

func TestMJBlendRequest_marshal(t *testing.T) {
	req := MJBlendRequest{
		ImageURLs:  []string{"a.png", "b.png"},
		Dimensions: "SQUARE",
		Speed:      "fast",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got map[string]any
	json.Unmarshal(data, &got)

	if len(got["image_urls"].([]any)) != 2 {
		t.Errorf("expected 2 image_urls, got %v", got["image_urls"])
	}
	if got["dimensions"] != "SQUARE" {
		t.Errorf("dimensions = %v, want SQUARE", got["dimensions"])
	}
}

// ============================================================================
// MJTaskActionRequest JSON marshaling
// ============================================================================

func TestMJTaskActionRequest_marshal(t *testing.T) {
	idx := 1
	req := MJTaskActionRequest{
		TaskID: "task_abc123",
		Index:  &idx,
		Speed:  "turbo",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var got map[string]any
	json.Unmarshal(data, &got)

	if got["task_id"] != "task_abc123" {
		t.Errorf("task_id = %v, want task_abc123", got["task_id"])
	}
	if got["index"] != float64(1) {
		t.Errorf("index = %v, want 1", got["index"])
	}
	if got["speed"] != "turbo" {
		t.Errorf("speed = %v, want turbo", got["speed"])
	}
}

func TestMJTaskActionRequest_customIDWins(t *testing.T) {
	idx := 2
	req := MJTaskActionRequest{
		TaskID:   "task_abc",
		Index:    &idx,
		CustomID: "MJ::JOB::upsample::2::xyz",
	}

	data, _ := json.Marshal(req)
	var got map[string]any
	json.Unmarshal(data, &got)

	if got["custom_id"] != "MJ::JOB::upsample::2::xyz" {
		t.Errorf("custom_id = %v, want MJ::JOB::upsample::2::xyz", got["custom_id"])
	}
	// Both index and custom_id should be present in JSON (API decides precedence)
	if got["index"] == nil {
		t.Error("index should also be present in JSON")
	}
}

// ============================================================================
// MJDescribeRequest & MJRerollRequest
// ============================================================================

func TestMJDescribeRequest_marshal(t *testing.T) {
	req := MJDescribeRequest{
		ImageURLs: []string{"https://example.com/img.png"},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if !containsJSONKey(data, "image_urls") {
		t.Error("describe request should contain image_urls")
	}
}

func TestMJRerollRequest_marshal(t *testing.T) {
	req := MJRerollRequest{
		TaskID: "task_abc",
	}
	data, _ := json.Marshal(req)
	if !containsJSONKey(data, "task_id") {
		t.Error("reroll request should contain task_id")
	}
	if containsJSONKey(data, "index") {
		t.Error("reroll request should NOT contain index")
	}
}

// ============================================================================
// MJTaskData unmarshal (from real API response)
// ============================================================================

func TestMJTaskData_unmarshal(t *testing.T) {
	src := `{
		"id": "task_01JWXXXX",
		"status": "SUCCESS",
		"action": "IMAGINE",
		"progress": "100%",
		"grid_image_url": "https://cdn.apimart.ai/grid.png",
		"image_urls": [
			"https://cdn.apimart.ai/img_0.png",
			"https://cdn.apimart.ai/img_1.png",
			"https://cdn.apimart.ai/img_2.png",
			"https://cdn.apimart.ai/img_3.png"
		],
		"buttons": [
			{"customId": "MJ::JOB::upsample::1::abc", "label": "U1"},
			{"customId": "MJ::JOB::variation::1::abc", "label": "V1"}
		],
		"prompt": "a beautiful sunset over mountains"
	}`

	var task MJTaskData
	if err := json.Unmarshal([]byte(src), &task); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if task.ID != "task_01JWXXXX" {
		t.Errorf("ID = %q, want task_01JWXXXX", task.ID)
	}
	if task.Status != "SUCCESS" {
		t.Errorf("Status = %q, want SUCCESS", task.Status)
	}
	if task.Action != "IMAGINE" {
		t.Errorf("Action = %q, want IMAGINE", task.Action)
	}
	if task.GridImageURL != "https://cdn.apimart.ai/grid.png" {
		t.Errorf("GridImageURL = %q", task.GridImageURL)
	}
	if len(task.ImageURLs) != 4 {
		t.Errorf("len(ImageURLs) = %d, want 4", len(task.ImageURLs))
	}
	if len(task.Buttons) != 2 {
		t.Errorf("len(Buttons) = %d, want 2", len(task.Buttons))
	}
	if task.Buttons[0].CustomID != "MJ::JOB::upsample::1::abc" {
		t.Errorf("Button[0].CustomID = %q", task.Buttons[0].CustomID)
	}
	if task.Prompt != "a beautiful sunset over mountains" {
		t.Errorf("Prompt = %q", task.Prompt)
	}
}

func TestMJTaskData_failure(t *testing.T) {
	src := `{
		"id": "task_fail",
		"status": "FAILURE",
		"fail_reason": "Banned prompt detected"
	}`

	var task MJTaskData
	if err := json.Unmarshal([]byte(src), &task); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if task.Status != "FAILURE" {
		t.Errorf("Status = %q, want FAILURE", task.Status)
	}
	if task.FailReason != "Banned prompt detected" {
		t.Errorf("FailReason = %q", task.FailReason)
	}
}

func TestMJTaskData_videoResponse(t *testing.T) {
	src := `{
		"id": "task_video",
		"status": "SUCCESS",
		"action": "VIDEO",
		"video_url": "https://r2.example.com/video-0.mp4",
		"video_urls": [
			"https://r2.example.com/video-0.mp4",
			"https://r2.example.com/video-1.mp4"
		]
	}`

	var task MJTaskData
	if err := json.Unmarshal([]byte(src), &task); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if task.VideoURL != "https://r2.example.com/video-0.mp4" {
		t.Errorf("VideoURL = %q", task.VideoURL)
	}
	if len(task.VideoURLs) != 2 {
		t.Errorf("len(VideoURLs) = %d, want 2", len(task.VideoURLs))
	}
}

func TestMJTaskData_describeResponse(t *testing.T) {
	src := `{
		"id": "task_desc",
		"status": "SUCCESS",
		"action": "DESCRIBE",
		"mode": "DESCRIBE",
		"prompt": "1️⃣ a serene mountain lake\n2️⃣ mountain landscape --v 6.1\n3️⃣ panoramic view --ar 16:9\n4️⃣ dawn light --s 250",
		"description": "1️⃣ a serene mountain lake\n2️⃣ mountain landscape --v 6.1\n3️⃣ panoramic view --ar 16:9\n4️⃣ dawn light --s 250"
	}`

	var task MJTaskData
	if err := json.Unmarshal([]byte(src), &task); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if task.Action != "DESCRIBE" {
		t.Errorf("Action = %q, want DESCRIBE", task.Action)
	}
	if task.Description == "" {
		t.Error("Description should not be empty")
	}
}

// ============================================================================
// MJSubmitResponse unmarshal
// ============================================================================

func TestMJSubmitResponse_unmarshal(t *testing.T) {
	src := `{
		"code": 200,
		"data": [{
			"status": "submitted",
			"task_id": "task_01KV52C0TEJSYZMCG0NCS4YWKK"
		}]
	}`

	var resp MJSubmitResponse
	if err := json.Unmarshal([]byte(src), &resp); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if resp.Code != 200 {
		t.Errorf("Code = %d, want 200", resp.Code)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("len(Data) = %d, want 1", len(resp.Data))
	}
	if resp.Data[0].TaskID != "task_01KV52C0TEJSYZMCG0NCS4YWKK" {
		t.Errorf("TaskID = %q", resp.Data[0].TaskID)
	}
	if resp.Data[0].Status != "submitted" {
		t.Errorf("Status = %q", resp.Data[0].Status)
	}
}

func TestMJSubmitResponse_empty(t *testing.T) {
	src := `{"code": 500, "data": []}`
	var resp MJSubmitResponse
	if err := json.Unmarshal([]byte(src), &resp); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if resp.Code != 500 {
		t.Errorf("Code = %d, want 500", resp.Code)
	}
	if len(resp.Data) != 0 {
		t.Errorf("len(Data) = %d, want 0", len(resp.Data))
	}
}

// ============================================================================
// MJVideoRequest / MJZoomRequest / MJPanRequest / MJModalRequest / MJRemixRequest
// ============================================================================

func TestMJVideoRequest_marshal(t *testing.T) {
	bs := 4
	req := MJVideoRequest{
		Prompt:    "cat video",
		ImageURLs: []string{"cat.png"},
		Motion:    "high",
		BatchSize: &bs,
	}

	data, _ := json.Marshal(req)
	var got map[string]any
	json.Unmarshal(data, &got)

	if got["prompt"] != "cat video" {
		t.Errorf("prompt = %v", got["prompt"])
	}
	if got["batch_size"] != float64(4) {
		t.Errorf("batch_size = %v, want 4", got["batch_size"])
	}
}

func TestMJZoomRequest_marshal(t *testing.T) {
	zr := 1.5
	req := MJZoomRequest{
		TaskID:    "task_abc",
		ZoomRatio: &zr,
	}

	data, _ := json.Marshal(req)
	var got map[string]any
	json.Unmarshal(data, &got)

	if got["task_id"] != "task_abc" {
		t.Errorf("task_id = %v", got["task_id"])
	}
	if got["zoom_ratio"] != float64(1.5) {
		t.Errorf("zoom_ratio = %v, want 1.5", got["zoom_ratio"])
	}
}

func TestMJPanRequest_marshal(t *testing.T) {
	req := MJPanRequest{
		TaskID:    "task_abc",
		Direction: "right",
	}

	data, _ := json.Marshal(req)
	var got map[string]any
	json.Unmarshal(data, &got)

	if got["direction"] != "right" {
		t.Errorf("direction = %v, want right", got["direction"])
	}
}

func TestMJModalRequest_marshal(t *testing.T) {
	req := MJModalRequest{
		TaskID:  "task_modal",
		Prompt:  "replace with sofa",
		MaskURL: "https://example.com/mask.png",
	}

	data, _ := json.Marshal(req)
	var got map[string]any
	json.Unmarshal(data, &got)

	if got["mask_url"] != "https://example.com/mask.png" {
		t.Errorf("mask_url = %v", got["mask_url"])
	}
}

func TestMJRemixRequest_marshal(t *testing.T) {
	idx := 1
	req := MJRemixRequest{
		TaskID: "task_v8",
		Index:  &idx,
		Prompt: "new style",
		Speed:  "fast",
	}

	data, _ := json.Marshal(req)
	var got map[string]any
	json.Unmarshal(data, &got)

	if got["prompt"] != "new style" {
		t.Errorf("prompt = %v", got["prompt"])
	}
	if got["speed"] != "fast" {
		t.Errorf("speed = %v", got["speed"])
	}
}

func TestMJRemixRequest_omitOptional(t *testing.T) {
	req := MJRemixRequest{
		TaskID: "task_v8",
	}

	data, _ := json.Marshal(req)
	if containsJSONKey(data, "prompt") {
		t.Error("prompt should be omitted when empty")
	}
	if containsJSONKey(data, "speed") {
		t.Error("speed should be omitted when empty")
	}
}

// ============================================================================
// MergeIntoImagine
// ============================================================================

func TestMergeIntoImagine_nil(t *testing.T) {
	var d *MidjourneyDefaults
	req := &MJImagineRequest{Prompt: "test"}
	d.MergeIntoImagine(req)
	if req.Prompt != "test" {
		t.Error("MergeIntoImagine on nil should not change anything")
	}
}

func TestMergeIntoImagine_emptyRequest(t *testing.T) {
	d := &MidjourneyDefaults{
		Speed:   "fast",
		Version: "6.1",
		Style:   "raw",
		Size:    "16:9",
		Quality: "1",
	}

	req := &MJImagineRequest{Prompt: "test"}
	d.MergeIntoImagine(req)

	if req.Speed != "fast" {
		t.Errorf("Speed = %q, want fast", req.Speed)
	}
	if req.Version != "6.1" {
		t.Errorf("Version = %q, want 6.1", req.Version)
	}
	if req.Style != "raw" {
		t.Errorf("Style = %q, want raw", req.Style)
	}
	if req.Size != "16:9" {
		t.Errorf("Size = %q, want 16:9", req.Size)
	}
	if req.Quality != "1" {
		t.Errorf("Quality = %q, want 1", req.Quality)
	}
}

func TestMergeIntoImagine_requestTakesPrecedence(t *testing.T) {
	d := &MidjourneyDefaults{
		Speed:   "relax",
		Version: "6.1",
		Size:    "1:1",
	}

	req := &MJImagineRequest{
		Prompt:  "test",
		Speed:   "fast",
		Version: "7",
	}
	d.MergeIntoImagine(req)

	if req.Speed != "fast" {
		t.Errorf("Speed should keep request value, got %q", req.Speed)
	}
	if req.Version != "7" {
		t.Errorf("Version should keep request value, got %q", req.Version)
	}
	if req.Size != "1:1" {
		t.Errorf("Size should be merged from defaults, got %q", req.Size)
	}
}

func TestMergeIntoImagine_niji(t *testing.T) {
	niji := true
	d := &MidjourneyDefaults{
		Niji: &niji,
	}

	req := &MJImagineRequest{Prompt: "anime"}
	d.MergeIntoImagine(req)

	if req.Niji == nil || *req.Niji != true {
		t.Error("Niji should be merged from defaults")
	}
}

func TestMergeIntoImagine_nijiRequestTakesPrecedence(t *testing.T) {
	niji := true
	d := &MidjourneyDefaults{
		Niji: &niji,
	}

	nijiFalse := false
	req := &MJImagineRequest{
		Prompt: "anime",
		Niji:   &nijiFalse,
	}
	d.MergeIntoImagine(req)

	if *req.Niji != false {
		t.Error("Request Niji=false should take precedence over defaults")
	}
}

// ============================================================================
// Helpers
// ============================================================================

func containsJSONKey(data []byte, key string) bool {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	_, ok := m[key]
	return ok
}
