package types

// MusicDefaults holds default values for music generation.
// Note: provider-specific request shape and last-resort values (suno vs
// flowmusic fields) live in code, not here.
type MusicDefaults struct {
	Provider     string `mapstructure:"provider" yaml:"provider,omitempty"`
	Model        string `mapstructure:"model" yaml:"model,omitempty"`
	Instrumental *bool  `mapstructure:"instrumental" yaml:"instrumental,omitempty"`
	Duration     *int   `mapstructure:"duration" yaml:"duration,omitempty"`
	Format       string `mapstructure:"format" yaml:"format,omitempty"`
	Timeout      *int   `mapstructure:"timeout" yaml:"timeout,omitempty"`
}

// MergeIntoMusic applies non-zero default values to a music generation request.
// Only empty/nil fields are filled — explicit request values always win.
func (d *MusicDefaults) MergeIntoMusic(req *MusicGenerateRequest) {
	if d == nil || req == nil {
		return
	}
	if req.Model == "" && d.Model != "" {
		req.Model = d.Model
	}
	if req.Instrumental == nil && d.Instrumental != nil {
		req.Instrumental = d.Instrumental
	}
	if req.Duration == nil && d.Duration != nil {
		req.Duration = d.Duration
	}
	if req.Format == "" && d.Format != "" {
		req.Format = d.Format
	}
}

// MusicGenerateRequest is the typed, backend-agnostic music request.
// Extras carries the --json overlay and is merged last (its keys win).
type MusicGenerateRequest struct {
	Model        string         `json:"model"`
	Prompt       string         `json:"prompt,omitempty"`
	Title        string         `json:"title,omitempty"`
	Style        string         `json:"style,omitempty"`
	Lyrics       string         `json:"lyrics,omitempty"`
	Instrumental *bool          `json:"instrumental,omitempty"`
	Duration     *int           `json:"duration,omitempty"`
	Format       string         `json:"format,omitempty"`
	Extras       map[string]any `json:"-"`
}

// MusicSubmitResponse is the POST /v1/music/generations response. Data is an array.
type MusicSubmitResponse struct {
	Code int                   `json:"code"`
	Data []MusicTaskSubmission `json:"data"`
}

// MusicTaskSubmission represents a submitted music task.
type MusicTaskSubmission struct {
	Status string `json:"status"`
	TaskID string `json:"task_id"`
}

// MusicTaskResponse is the GET /v1/music/tasks/{task_id} response. Data is an object.
type MusicTaskResponse struct {
	Code int           `json:"code"`
	Data MusicTaskData `json:"data"`
}

// MusicTaskData contains the full music task info.
type MusicTaskData struct {
	ID          string           `json:"id"`
	Status      string           `json:"status"`
	Progress    int              `json:"progress"`
	Created     int64            `json:"created"`
	Completed   int64            `json:"completed"`
	ActualTime  int              `json:"actual_time"`
	Cost        float64          `json:"cost"`
	CreditsCost float64          `json:"credits_cost"`
	Result      *MusicTaskResult `json:"result"`
	Error       *MusicTaskError  `json:"error"`
}

// MusicTaskResult wraps the generated tracks.
type MusicTaskResult struct {
	Music []MusicTrack `json:"music"`
}

// MusicTrack is the union of the suno and flowmusic track shapes.
// DurationSeconds is a string because the flowmusic backend returns it as one.
type MusicTrack struct {
	AudioID         string  `json:"audio_id,omitempty"`
	ClipID          string  `json:"clip_id,omitempty"`
	Title           string  `json:"title,omitempty"`
	Lyrics          string  `json:"lyrics,omitempty"`
	Tags            string  `json:"tags,omitempty"`
	DurationSeconds string  `json:"duration_seconds,omitempty"`
	AudioURL        string  `json:"audio_url,omitempty"`
	WAVURL          string  `json:"wav_url,omitempty"`
	ImageURL        string  `json:"image_url,omitempty"`
	VideoURL        string  `json:"video_url,omitempty"`
	FileURL         string  `json:"file_url,omitempty"`
	Duration        float64 `json:"duration,omitempty"`
}

// MusicTaskError describes a failed music task.
type MusicTaskError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

// ============================================================================
// OpenRouter music (Google Lyria) — synchronous SSE over /chat/completions
// ============================================================================

// OpenRouterMusicRequest is the sync streaming music request body.
type OpenRouterMusicRequest struct {
	Model      string                   `json:"model"`
	Messages   []OpenRouterMusicMessage `json:"messages"`
	Modalities []string                 `json:"modalities"`
	Audio      *OpenRouterAudioConfig   `json:"audio,omitempty"`
	Stream     bool                     `json:"stream"`
}

// OpenRouterMusicMessage is a single chat message carrying the music prompt.
type OpenRouterMusicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenRouterAudioConfig configures the requested audio output format.
type OpenRouterAudioConfig struct {
	Format string `json:"format,omitempty"`
}
