package types

import (
	"encoding/json"
	"testing"
)

func TestOpenRouterMediaModelList_unmarshal(t *testing.T) {
	data := `{
		"data": [
			{
				"id": "openai/gpt-image-2",
				"name": "OpenAI: GPT Image 2",
				"description": "OpenAI's image generation model.",
				"created": 1782264714,
				"architecture": {
					"input_modalities": ["text", "image"],
					"output_modalities": ["image"]
				},
				"supported_parameters": {
					"quality": {
						"type": "enum",
						"values": ["auto", "low", "medium", "high"]
					},
					"n": {
						"type": "range",
						"min": 1,
						"max": 10
					},
					"seed": {
						"type": "boolean"
					}
				},
				"supports_streaming": true,
				"endpoints": "/api/v1/images/models/openai/gpt-image-2/endpoints"
			}
		]
	}`

	var list OpenRouterMediaModelList
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(list.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(list.Data))
	}

	m := list.Data[0]
	if m.ID != "openai/gpt-image-2" {
		t.Errorf("ID = %q, want %q", m.ID, "openai/gpt-image-2")
	}
	if m.Name != "OpenAI: GPT Image 2" {
		t.Errorf("Name = %q, want %q", m.Name, "OpenAI: GPT Image 2")
	}
	if m.Description != "OpenAI's image generation model." {
		t.Errorf("Description = %q", m.Description)
	}
	if m.Created != 1782264714 {
		t.Errorf("Created = %d", m.Created)
	}
	if !m.SupportsStreaming {
		t.Error("SupportsStreaming should be true")
	}
	if m.Endpoints != "/api/v1/images/models/openai/gpt-image-2/endpoints" {
		t.Errorf("Endpoints = %q", m.Endpoints)
	}

	// Architecture
	if m.Architecture == nil {
		t.Fatal("Architecture should not be nil")
	}
	if len(m.Architecture.InputModalities) != 2 || m.Architecture.InputModalities[0] != "text" {
		t.Errorf("InputModalities = %v", m.Architecture.InputModalities)
	}
	if len(m.Architecture.OutputModalities) != 1 || m.Architecture.OutputModalities[0] != "image" {
		t.Errorf("OutputModalities = %v", m.Architecture.OutputModalities)
	}

	// Parameters
	if len(m.SupportedParameters) != 3 {
		t.Fatalf("expected 3 parameters, got %d", len(m.SupportedParameters))
	}

	// Enum parameter
	q, ok := m.SupportedParameters["quality"]
	if !ok {
		t.Fatal("missing quality parameter")
	}
	if q.Type != "enum" {
		t.Errorf("quality.Type = %q, want %q", q.Type, "enum")
	}
	if len(q.Values) != 4 || q.Values[0] != "auto" {
		t.Errorf("quality.Values = %v", q.Values)
	}

	// Range parameter
	n, ok := m.SupportedParameters["n"]
	if !ok {
		t.Fatal("missing n parameter")
	}
	if n.Type != "range" {
		t.Errorf("n.Type = %q, want %q", n.Type, "range")
	}
	if n.Min == nil || *n.Min != 1 {
		t.Errorf("n.Min = %v", n.Min)
	}
	if n.Max == nil || *n.Max != 10 {
		t.Errorf("n.Max = %v", n.Max)
	}

	// Boolean parameter
	s, ok := m.SupportedParameters["seed"]
	if !ok {
		t.Fatal("missing seed parameter")
	}
	if s.Type != "boolean" {
		t.Errorf("seed.Type = %q, want %q", s.Type, "boolean")
	}
}

func TestOpenRouterMediaModelList_empty(t *testing.T) {
	data := `{"data": []}`
	var list OpenRouterMediaModelList
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		t.Fatalf("failed to unmarshal empty list: %v", err)
	}
	if len(list.Data) != 0 {
		t.Errorf("expected empty list, got %d items", len(list.Data))
	}
}

func TestOpenRouterVideoSubmitResponse_unmarshal(t *testing.T) {
	data := `{
		"id": "vid_abc123",
		"polling_url": "https://openrouter.ai/api/v1/videos/vid_abc123/poll",
		"status": "pending"
	}`

	var resp OpenRouterVideoSubmitResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.ID != "vid_abc123" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.PollingURL != "https://openrouter.ai/api/v1/videos/vid_abc123/poll" {
		t.Errorf("PollingURL = %q", resp.PollingURL)
	}
	if resp.Status != "pending" {
		t.Errorf("Status = %q", resp.Status)
	}
}

func TestOpenRouterVideoStatusResponse_completed(t *testing.T) {
	data := `{
		"id": "vid_abc123",
		"status": "completed",
		"unsigned_urls": ["https://cdn.openrouter.ai/videos/vid_abc123.mp4"],
		"usage": {
			"input_tokens": 50,
			"output_tokens": 8000,
			"total_cost": 0.15
		}
	}`

	var resp OpenRouterVideoStatusResponse
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Status != "completed" {
		t.Errorf("Status = %q", resp.Status)
	}
	if len(resp.UnsignedURLs) != 1 || resp.UnsignedURLs[0] != "https://cdn.openrouter.ai/videos/vid_abc123.mp4" {
		t.Errorf("UnsignedURLs = %v", resp.UnsignedURLs)
	}
	if resp.Usage == nil {
		t.Fatal("Usage should not be nil")
	}
	if resp.Usage.InputTokens != 50 {
		t.Errorf("InputTokens = %d", resp.Usage.InputTokens)
	}
	if resp.Usage.TotalCost != 0.15 {
		t.Errorf("TotalCost = %f", resp.Usage.TotalCost)
	}
}
