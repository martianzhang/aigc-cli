package types

// AudioSpeechRequest is the request body for POST /v1/audio/speech (TTS).
// Compatible with OpenAI, OpenRouter, and APIMart.
type AudioSpeechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format,omitempty"`
	Speed          float64 `json:"speed,omitempty"`
	Instructions   string  `json:"instructions,omitempty"` // OpenAI gpt-4o-mini-tts only
}

// AudioTranscribeRequest is the request body for POST /v1/audio/transcriptions (STT).
// Uses base64-encoded audio in JSON body (alternative to multipart).
type AudioTranscribeRequest struct {
	Model       string      `json:"model"`
	InputAudio  *AudioInput `json:"input_audio"`
	Language    string      `json:"language,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
}

// AudioInput holds base64-encoded audio data and its format.
type AudioInput struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

// AudioTranscribeResponse is the JSON response from the STT endpoint.
type AudioTranscribeResponse struct {
	Text  string      `json:"text"`
	Usage *AudioUsage `json:"usage,omitempty"`
}

// AudioUsage holds usage statistics from STT responses.
type AudioUsage struct {
	Seconds      float64 `json:"seconds"`
	TotalTokens  int     `json:"total_tokens"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	Cost         float64 `json:"cost"`
}
