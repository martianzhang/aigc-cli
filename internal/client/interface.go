package client

import (
	"time"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// APIClient defines the interface for API operations.
// Implementations: Client (production), mock clients (testing).
// Adding a new method here? Make sure *Client implements it and
// update callers in cmd/ and internal/mcp/ if they depend on the interface.
type APIClient interface {
	BaseURL() string
	SetTimeout(d time.Duration)
	Submit(req *types.GenerateRequest) (*types.GenerateResponse, error)
	ImageGenerateSync(req *types.GenerateRequest) (*types.OpenAIImageResponse, error)
	PollTask(taskID string) (*types.TaskData, error)
	GetTask(taskID string) (*types.TaskData, error)
	ResolveLocalImages(urls []string) ([]string, error)
	ChatCompletion(req *types.ChatRequest) (*types.ChatResponse, error)
	VideoSubmit(req *types.VideoGenerateRequest) (*types.VideoGenerateResponse, error)
	GetTokenBalance() (*types.TokenBalanceResponse, error)
	GetUserBalance() (*types.UserBalanceResponse, error)
	ListModelsOpenAI() ([]types.OpenAIModel, error)
	GetModelOpenAI(modelID string) (*types.OpenAIModel, error)

	// Provider-specific helpers used in cmd/
	OpenRouterDedicatedImage(req *types.GenerateRequest) (*types.OpenAIImageResponse, error)
	OpenRouterVideoSubmit(req *types.OpenRouterVideoRequest) (*types.OpenRouterVideoSubmitResponse, error)
	OpenRouterVideoPoll(pollingURL string) (*types.OpenRouterVideoStatusResponse, error)
	OpenRouterVideoGet(jobID string) (*types.OpenRouterVideoStatusResponse, error)
	OpenRouterVideoPollUntilComplete(pollingURL string, pollInterval, maxWait time.Duration) (*types.OpenRouterVideoStatusResponse, error)
	YunwuVideoSubmit(req *types.VideoGenerateRequest) (*types.YunwuVideoCreateResponse, error)
	YunwuVideoQuery(taskID string) (*types.YunwuVideoQueryResponse, error)
	MidjourneySubmit(action string, reqBody any) (*types.MJSubmitResponse, error)
	MidjourneyGetTask(taskID string) (*types.MJTaskData, error)
	MidjourneyPollTask(taskID string) (*types.MJTaskData, error)

	// Audio (TTS/STT)
	AudioSpeech(req *types.AudioSpeechRequest) ([]byte, string, error)
	AudioTranscribe(req *types.AudioTranscribeRequest) (*types.AudioTranscribeResponse, error)
	AudioTranscribeMultipart(model, filePath, language string) (*types.AudioTranscribeResponse, error)
}
