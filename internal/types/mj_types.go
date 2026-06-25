// Package types defines request/response data structures for the Midjourney API.
package types

// ============================================================================
// MJ submission response (shared by all POST /v1/midjourney/generations/* endpoints)
// ============================================================================

// MJSubmitResponse is the standard submission response for all MJ POST endpoints.
type MJSubmitResponse struct {
	Code int                `json:"code"`
	Data []MJTaskSubmission `json:"data"`
}

// MJTaskSubmission represents a submitted MJ task.
type MJTaskSubmission struct {
	Status string `json:"status"`
	TaskID string `json:"task_id"`
}

// ============================================================================
// MJ-specific task query response (GET /v1/midjourney/{task_id})
// ============================================================================

// MJTaskData contains the full MJ task info from GET /v1/midjourney/{task_id}.
type MJTaskData struct {
	ID           string     `json:"id"`
	Status       string     `json:"status"`
	Action       string     `json:"action,omitempty"`
	Mode         string     `json:"mode,omitempty"`
	Progress     string     `json:"progress,omitempty"`
	GridImageURL string     `json:"grid_image_url,omitempty"`
	ImageURLs    []string   `json:"image_urls,omitempty"`
	VideoURL     string     `json:"video_url,omitempty"`
	VideoURLs    []string   `json:"video_urls,omitempty"`
	Prompt       string     `json:"prompt,omitempty"`
	Description  string     `json:"description,omitempty"`
	Buttons      []MJButton `json:"buttons,omitempty"`
	FailReason   string     `json:"fail_reason,omitempty"`
	Cost         float64    `json:"cost,omitempty"`
	CreditsCost  float64    `json:"credits_cost,omitempty"`
	ActualTime   int        `json:"actual_time,omitempty"`
	Created      int64      `json:"created,omitempty"`
	Completed    int64      `json:"completed,omitempty"`
}

// MJButton represents a follow-up action button in an MJ task result.
type MJButton struct {
	CustomID string `json:"customId"`
	Label    string `json:"label"`
	Style    int    `json:"style,omitempty"`
}

// ============================================================================
// Request types for each MJ endpoint
// ============================================================================

// ---- Imagine (POST /v1/midjourney/generations[/imagine]) ----

// MJImagineRequest is the request body for imagine (text-to-image / image-guided).
type MJImagineRequest struct {
	Prompt    string            `json:"prompt"`
	ImageURLs []string          `json:"image_urls,omitempty"`
	Speed     string            `json:"speed,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`

	// Structured fields (override prompt flags)
	Size           string   `json:"size,omitempty"`
	Quality        string   `json:"quality,omitempty"`
	Style          string   `json:"style,omitempty"`
	Version        string   `json:"version,omitempty"`
	Seed           *int     `json:"seed,omitempty"`
	NegativePrompt string   `json:"negative_prompt,omitempty"`
	Stylize        *int     `json:"stylize,omitempty"`
	Chaos          *int     `json:"chaos,omitempty"`
	Weird          *int     `json:"weird,omitempty"`
	Tile           *bool    `json:"tile,omitempty"`
	Niji           *bool    `json:"niji,omitempty"`
	Iw             *float64 `json:"iw,omitempty"`
	Cw             *int     `json:"cw,omitempty"`
	Sw             *int     `json:"sw,omitempty"`
	Cref           string   `json:"cref,omitempty"`
	Sref           string   `json:"sref,omitempty"`
	Dref           string   `json:"dref,omitempty"`
	Dw             *float64 `json:"dw,omitempty"`
	Repeat         *int     `json:"repeat,omitempty"`
	Raw            *bool    `json:"raw,omitempty"`
	Draft          *bool    `json:"draft,omitempty"`
	Hd             *bool    `json:"hd,omitempty"`
	Stop           *int     `json:"stop,omitempty"`
	Extra          string   `json:"extra,omitempty"`
}

// ---- Blend (POST /v1/midjourney/generations/blend) ----

// MJBlendRequest is the request body for blend (multi-image blend).
type MJBlendRequest struct {
	ImageURLs  []string          `json:"image_urls"`
	Dimensions string            `json:"dimensions,omitempty"` // SQUARE / PORTRAIT / LANDSCAPE
	Size       string            `json:"size,omitempty"`
	Speed      string            `json:"speed,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// ---- Describe (POST /v1/midjourney/generations/describe) ----

// MJDescribeRequest is the request body for describe (image to text).
type MJDescribeRequest struct {
	ImageURLs []string          `json:"image_urls"`
	Speed     string            `json:"speed,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ---- Edits (POST /v1/midjourney/generations/edits) ----

// MJEditsRequest inherits the same fields as Imagine (prompt + image_urls + structured).
// Uses MJImagineRequest directly.

// ---- Upscale / Variation / High Variation / Low Variation / Inpaint ----

// MJTaskActionRequest is shared by upscale / variation / high-variation / low-variation / inpaint.
type MJTaskActionRequest struct {
	TaskID   string            `json:"task_id"`
	Index    *int              `json:"index,omitempty"`
	CustomID string            `json:"custom_id,omitempty"`
	Speed    string            `json:"speed,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ---- Reroll (POST /v1/midjourney/generations/reroll) ----

// MJRerollRequest is the request body for reroll (no index needed).
type MJRerollRequest struct {
	TaskID   string            `json:"task_id"`
	CustomID string            `json:"custom_id,omitempty"`
	Speed    string            `json:"speed,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ---- Zoom (POST /v1/midjourney/generations/zoom) ----

// MJZoomRequest is the request body for zoom.
type MJZoomRequest struct {
	TaskID    string            `json:"task_id"`
	CustomID  string            `json:"custom_id,omitempty"`
	Index     *int              `json:"index,omitempty"`
	ZoomRatio *float64          `json:"zoom_ratio,omitempty"`
	Speed     string            `json:"speed,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ---- Pan (POST /v1/midjourney/generations/pan) ----

// MJPanRequest is the request body for pan.
type MJPanRequest struct {
	TaskID    string            `json:"task_id"`
	CustomID  string            `json:"custom_id,omitempty"`
	Index     *int              `json:"index,omitempty"`
	Direction string            `json:"direction,omitempty"` // left / right / up / down
	Speed     string            `json:"speed,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// ---- Modal (POST /v1/midjourney/generations/modal) ----

// MJModalRequest is the request body for modal (submit inpaint parameters).
type MJModalRequest struct {
	TaskID   string            `json:"task_id"`
	Prompt   string            `json:"prompt,omitempty"`
	MaskURL  string            `json:"mask_url,omitempty"`
	Speed    string            `json:"speed,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ---- Video (POST /v1/midjourney/generations/video) ----

// MJVideoRequest is the request body for MJ image-to-video.
type MJVideoRequest struct {
	Prompt      string   `json:"prompt,omitempty"`
	ImageURLs   []string `json:"image_urls,omitempty"`
	TaskID      string   `json:"task_id,omitempty"`
	Index       *int     `json:"index,omitempty"`
	VideoType   string   `json:"video_type,omitempty"`
	AnimateMode string   `json:"animate_mode,omitempty"`
	Motion      string   `json:"motion,omitempty"`
	BatchSize   *int     `json:"batch_size,omitempty"`
	EndURL      string   `json:"end_url,omitempty"`
}

// ---- Remix (POST /v1/midjourney/generations/remix-strong / remix-subtle) ----

// MJRemixRequest is the request body for remix.
type MJRemixRequest struct {
	TaskID string `json:"task_id"`
	Index  *int   `json:"index"`
	Prompt string `json:"prompt,omitempty"`
	Speed  string `json:"speed,omitempty"`
}
