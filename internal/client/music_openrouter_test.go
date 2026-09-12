package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestOpenRouterMusicGenerate_streamsAudio(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody types.OpenRouterMusicRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)

		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"audio\":{\"data\":\"aGV\",\"transcript\":\"hel\"}}}]}\n\n"))
		w.Write([]byte("data: {\"choices\":[{\"delta\":{\"audio\":{\"data\":\"sbG8=\",\"transcript\":\"lo\"}}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	c := New("sk-test", srv.URL, "")
	audio, transcript, err := c.OpenRouterMusicGenerate(&types.OpenRouterMusicRequest{
		Model:      "google/lyria-3-clip-preview",
		Messages:   []types.OpenRouterMusicMessage{{Role: "user", Content: "lofi"}},
		Modalities: []string{"text", "audio"},
		Audio:      &types.OpenRouterAudioConfig{Format: "mp3"},
		Stream:     true,
	})
	if err != nil {
		t.Fatalf("OpenRouterMusicGenerate() error = %v", err)
	}
	if string(audio) != "hello" {
		t.Errorf("audio = %q, want %q", string(audio), "hello")
	}
	if transcript != "hello" {
		t.Errorf("transcript = %q, want %q", transcript, "hello")
	}
	if gotPath != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if !gotBody.Stream {
		t.Error("request stream should be true")
	}
	if len(gotBody.Modalities) != 2 || gotBody.Modalities[1] != "audio" {
		t.Errorf("modalities = %v", gotBody.Modalities)
	}
}

func TestOpenRouterMusicGenerate_noAudioErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data: {\"choices\":[{\"delta\":{}}]}\n\n"))
		w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	c := New("sk-test", srv.URL, "")
	_, _, err := c.OpenRouterMusicGenerate(&types.OpenRouterMusicRequest{Stream: true})
	if err == nil {
		t.Fatal("expected error when no audio data received")
	}
}
