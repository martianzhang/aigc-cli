package mcp

import (
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/martianzhang/apimart-cli/internal/client"
	"github.com/martianzhang/apimart-cli/internal/types"
)

// mockAPIClient implements client.APIClient for testing MCP handlers.
type mockAPIClient struct {
	getTaskFn               func(taskID string) (*types.TaskData, error)
	openRouterVideoGetFn    func(jobID string) (*types.OpenRouterVideoStatusResponse, error)
	openRouterVideoSubmitFn func(req *types.OpenRouterVideoRequest) (*types.OpenRouterVideoSubmitResponse, error)
}

func (m *mockAPIClient) SetTimeout(d time.Duration) {}
func (m *mockAPIClient) Submit(req *types.GenerateRequest) (*types.GenerateResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) ImageGenerateSync(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) PollTask(taskID string) (*types.TaskData, error) { return nil, nil }
func (m *mockAPIClient) GetTask(taskID string) (*types.TaskData, error) {
	if m.getTaskFn != nil {
		return m.getTaskFn(taskID)
	}
	return &types.TaskData{ID: taskID, Status: "completed", Progress: 100}, nil
}
func (m *mockAPIClient) ResolveLocalImages(urls []string) ([]string, error) { return urls, nil }
func (m *mockAPIClient) ChatCompletion(req *types.ChatRequest) (*types.ChatResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) VideoSubmit(req *types.VideoGenerateRequest) (*types.VideoGenerateResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) GetTokenBalance() (*types.TokenBalanceResponse, error) {
	return &types.TokenBalanceResponse{Success: true, RemainBalance: 100}, nil
}
func (m *mockAPIClient) GetUserBalance() (*types.UserBalanceResponse, error) {
	return &types.UserBalanceResponse{Success: true, RemainBalance: 100}, nil
}
func (m *mockAPIClient) ListModelsOpenAI() ([]types.OpenAIModel, error) {
	return []types.OpenAIModel{{ID: "test-model"}}, nil
}
func (m *mockAPIClient) GetModelOpenAI(modelID string) (*types.OpenAIModel, error) {
	return &types.OpenAIModel{ID: modelID, OwnedBy: "test"}, nil
}
func (m *mockAPIClient) OpenRouterDedicatedImage(req *types.GenerateRequest) (*types.OpenAIImageResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) OpenRouterVideoSubmit(req *types.OpenRouterVideoRequest) (*types.OpenRouterVideoSubmitResponse, error) {
	if m.openRouterVideoSubmitFn != nil {
		return m.openRouterVideoSubmitFn(req)
	}
	return &types.OpenRouterVideoSubmitResponse{ID: "job_123", Status: "pending", PollingURL: "https://poll.url"}, nil
}
func (m *mockAPIClient) OpenRouterVideoPoll(pollingURL string) (*types.OpenRouterVideoStatusResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) OpenRouterVideoGet(jobID string) (*types.OpenRouterVideoStatusResponse, error) {
	if m.openRouterVideoGetFn != nil {
		return m.openRouterVideoGetFn(jobID)
	}
	return &types.OpenRouterVideoStatusResponse{ID: jobID, Status: "completed"}, nil
}
func (m *mockAPIClient) OpenRouterVideoPollUntilComplete(pollingURL string, pollInterval, maxWait time.Duration) (*types.OpenRouterVideoStatusResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) YunwuVideoSubmit(req *types.VideoGenerateRequest) (*types.YunwuVideoCreateResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) YunwuVideoQuery(taskID string) (*types.YunwuVideoQueryResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) MidjourneySubmit(action string, reqBody any) (*types.MJSubmitResponse, error) {
	return nil, nil
}
func (m *mockAPIClient) MidjourneyGetTask(taskID string) (*types.MJTaskData, error) {
	return nil, nil
}
func (m *mockAPIClient) MidjourneyPollTask(taskID string) (*types.MJTaskData, error) {
	return nil, nil
}

// compile-time check
var _ client.APIClient = (*mockAPIClient)(nil)

// --- Tests ---

func TestHandleMCPGetAPIMartTask_completed(t *testing.T) {
	mock := &mockAPIClient{
		getTaskFn: func(taskID string) (*types.TaskData, error) {
			return &types.TaskData{
				ID: taskID, Status: "completed", Progress: 100,
				Cost: 0.05, CreditsCost: 0.5, ActualTime: 30,
			}, nil
		},
	}
	result, err := handleMCPGetAPIMartTask(mock, "task_123", t.TempDir())
	if err != nil {
		t.Fatalf("handleMCPGetAPIMartTask returned error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !contains(text, "completed") {
		t.Errorf("result should mention completed, got: %s", text)
	}
	if !contains(text, "0.05") {
		t.Errorf("result should show cost, got: %s", text)
	}
}

func TestHandleMCPGetAPIMartTask_inProgress(t *testing.T) {
	mock := &mockAPIClient{
		getTaskFn: func(taskID string) (*types.TaskData, error) {
			return &types.TaskData{
				ID: taskID, Status: "in_progress", Progress: 50,
			}, nil
		},
	}
	result, err := handleMCPGetAPIMartTask(mock, "task_456", t.TempDir())
	if err != nil {
		t.Fatalf("handleMCPGetAPIMartTask returned error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !contains(text, "in_progress") {
		t.Errorf("result should show in_progress, got: %s", text)
	}
	if !contains(text, "50%") {
		t.Errorf("result should show 50%%, got: %s", text)
	}
}

func TestHandleMCPGetAPIMartTask_failed(t *testing.T) {
	mock := &mockAPIClient{
		getTaskFn: func(taskID string) (*types.TaskData, error) {
			return &types.TaskData{
				ID: taskID, Status: "failed", Progress: 30,
			}, nil
		},
	}
	result, err := handleMCPGetAPIMartTask(mock, "task_fail", t.TempDir())
	if err != nil {
		t.Fatalf("handleMCPGetAPIMartTask returned error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !contains(text, "failed") {
		t.Errorf("result should show failed, got: %s", text)
	}
}

func TestHandleMCPGetOpenRouterJob_completed(t *testing.T) {
	mock := &mockAPIClient{
		openRouterVideoGetFn: func(jobID string) (*types.OpenRouterVideoStatusResponse, error) {
			return &types.OpenRouterVideoStatusResponse{
				ID: jobID, Status: "completed",
				UnsignedURLs: []string{"https://example.com/video.mp4"},
				Usage:        &types.OpenRouterUsage{TotalCost: 0.1},
			}, nil
		},
	}
	result, err := handleMCPGetOpenRouterJob(mock, "job_789", t.TempDir(), "test-key")
	if err != nil {
		t.Fatalf("handleMCPGetOpenRouterJob returned error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !contains(text, "completed") {
		t.Errorf("result should mention completed, got: %s", text)
	}
	if !contains(text, "0.1") {
		t.Errorf("result should show cost, got: %s", text)
	}
}

func TestHandleMCPGetOpenRouterJob_pending(t *testing.T) {
	mock := &mockAPIClient{
		openRouterVideoGetFn: func(jobID string) (*types.OpenRouterVideoStatusResponse, error) {
			return &types.OpenRouterVideoStatusResponse{
				ID: jobID, Status: "pending",
			}, nil
		},
	}
	result, err := handleMCPGetOpenRouterJob(mock, "job_pending", t.TempDir(), "test-key")
	if err != nil {
		t.Fatalf("handleMCPGetOpenRouterJob returned error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !contains(text, "pending") {
		t.Errorf("result should show pending, got: %s", text)
	}
}

func TestHandleMCPGetOpenRouterJob_failed(t *testing.T) {
	mock := &mockAPIClient{
		openRouterVideoGetFn: func(jobID string) (*types.OpenRouterVideoStatusResponse, error) {
			return &types.OpenRouterVideoStatusResponse{
				ID: jobID, Status: "failed", Error: "rate limit exceeded",
			}, nil
		},
	}
	result, err := handleMCPGetOpenRouterJob(mock, "job_fail", t.TempDir(), "test-key")
	if err != nil {
		t.Fatalf("handleMCPGetOpenRouterJob returned error: %v", err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !contains(text, "rate limit") {
		t.Errorf("result should show error message, got: %s", text)
	}
}
