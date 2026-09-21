package video

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// videoOverlayFlags lists every video flag the overlay tests may set; clearing
// Changed between cases keeps them independent of execution order.
var videoOverlayFlags = []string{
	"model", "prompt", "duration", "size", "resolution", "seed",
	"generate-audio", "return-last-frame", "image-url", "video-url",
	"audio-url", "tool", "first-frame", "last-frame", "remix", "raw",
	"task-id", "job-id", "dry-run", "preview", "gif", "gif-width", "mp4",
	"crop-margin", "ffmpeg-flags", "json",
}

// videoTestCmd restores the shared video command and package flag state, then
// returns it ready for a table case. Its --model flag stands in for the root
// command's persistent flag, bound to options.Shared.Model the same way.
func videoTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	if videoCmd.Flags().Lookup("model") == nil {
		videoCmd.Flags().StringVar(&options.Shared.Model, "model", "", "test-only stand-in for the root persistent flag")
	}
	resetVideoOverlayState()
	t.Cleanup(resetVideoOverlayState)
	return videoCmd
}

// resetVideoOverlayState clears Changed bits and re-zeroes the bound package
// variables, so each table case starts from the flag defaults.
func resetVideoOverlayState() {
	for _, name := range videoOverlayFlags {
		if f := videoCmd.Flags().Lookup(name); f != nil {
			f.Changed = false
		}
	}
	vidPrompt, vidDuration, vidSize, vidResolution, vidSeed = "", 0, "", "", 0
	vidGenerateAudio, vidReturnLastFrame = false, false
	vidImageURLs, vidVideoURLs, vidAudioURLs, vidTools = nil, nil, nil, nil
	vidFirstFrame, vidLastFrame = "", ""
	options.Shared.JSONInput, options.Shared.Model = "", ""
}

func setVideoFlag(t *testing.T, name, value string) {
	t.Helper()
	if err := videoCmd.Flags().Set(name, value); err != nil {
		t.Fatalf("set --%s=%s: %v", name, value, err)
	}
}

// videoRequestBody decodes the merged RawJSON into a generic map.
func videoRequestBody(t *testing.T, req *types.VideoGenerateRequest) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(req.RawJSON, &body); err != nil {
		t.Fatalf("merged body is not a JSON object: %v", err)
	}
	return body
}

func TestBuildVideoRequestJSONFlagOverrides(t *testing.T) {
	const vendorBody = `{"model":"json-model","prompt":"json-prompt","size":"1:1","vendor_obj":{"n":1},"vendor_arr":[1,2]}`

	tests := []struct {
		name      string
		jsonBody  string
		set       func(t *testing.T)
		want      map[string]any
		absent    []string
		wantBytes bool
	}{
		{
			name:      "no flags leaves the JSON byte-identical",
			wantBytes: true,
			want:      map[string]any{"model": "json-model", "prompt": "json-prompt", "size": "1:1"},
		},
		{
			name: "model flag wins",
			set:  func(t *testing.T) { setVideoFlag(t, "model", "flag-model") },
			want: map[string]any{"model": "flag-model", "prompt": "json-prompt", "size": "1:1"},
		},
		{
			name: "prompt flag wins",
			set:  func(t *testing.T) { setVideoFlag(t, "prompt", "flag-prompt") },
			want: map[string]any{"model": "json-model", "prompt": "flag-prompt", "size": "1:1"},
		},
		{
			name: "size, duration, resolution and seed win",
			set: func(t *testing.T) {
				setVideoFlag(t, "size", "9:16")
				setVideoFlag(t, "duration", "6")
				setVideoFlag(t, "resolution", "720p")
				setVideoFlag(t, "seed", "42")
			},
			want: map[string]any{
				"model": "json-model", "prompt": "json-prompt", "size": "9:16",
				"duration": float64(6), "resolution": "720p", "seed": float64(42),
			},
		},
		{
			name:     "generate-audio true wins over JSON false",
			jsonBody: `{"model":"m","generate_audio":false}`,
			set:      func(t *testing.T) { setVideoFlag(t, "generate-audio", "true") },
			want:     map[string]any{"model": "m", "generate_audio": true},
		},
		{
			name:     "explicit generate-audio=false wins over JSON true",
			jsonBody: `{"model":"m","generate_audio":true}`,
			set:      func(t *testing.T) { setVideoFlag(t, "generate-audio", "false") },
			want:     map[string]any{"model": "m", "generate_audio": false},
		},
		{
			name:     "return-last-frame wins",
			jsonBody: `{"model":"m","return_last_frame":false}`,
			set:      func(t *testing.T) { setVideoFlag(t, "return-last-frame", "true") },
			want:     map[string]any{"model": "m", "return_last_frame": true},
		},
		{
			name:     "repeatable URL flags replace JSON arrays",
			jsonBody: `{"model":"m","image_urls":["json.png"],"video_urls":["json.mp4"],"audio_urls":["json.mp3"]}`,
			set: func(t *testing.T) {
				setVideoFlag(t, "image-url", "a.png")
				setVideoFlag(t, "image-url", "b.png")
				setVideoFlag(t, "video-url", "clip.mp4")
				setVideoFlag(t, "audio-url", "voice.mp3")
			},
			want: map[string]any{
				"model":      "m",
				"image_urls": []any{"a.png", "b.png"},
				"video_urls": []any{"clip.mp4"},
				"audio_urls": []any{"voice.mp3"},
			},
		},
		{
			name:     "tool flags replace JSON tools",
			jsonBody: `{"model":"m","tools":[{"type":"json_tool"}]}`,
			set: func(t *testing.T) {
				setVideoFlag(t, "tool", "web_search")
				setVideoFlag(t, "tool", "code")
			},
			want: map[string]any{
				"model": "m",
				"tools": []any{map[string]any{"type": "web_search"}, map[string]any{"type": "code"}},
			},
		},
		{
			name:     "unset flags add no keys",
			jsonBody: `{"model":"m"}`,
			set:      func(t *testing.T) { setVideoFlag(t, "model", "flag-model") },
			want:     map[string]any{"model": "flag-model"},
			absent: []string{
				"prompt", "duration", "size", "resolution", "seed",
				"generate_audio", "return_last_frame", "image_urls",
				"video_urls", "audio_urls", "tools", "image_with_roles",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := videoTestCmd(t)
			jsonBody := tc.jsonBody
			if jsonBody == "" {
				jsonBody = vendorBody
			}
			options.Shared.JSONInput = jsonBody
			if tc.set != nil {
				tc.set(t)
			}

			req, err := buildVideoRequest(cmd)
			if err != nil {
				t.Fatalf("buildVideoRequest: %v", err)
			}

			if tc.wantBytes && string(req.RawJSON) != jsonBody {
				t.Errorf("body must stay byte-identical:\n got %s\nwant %s", req.RawJSON, jsonBody)
			}
			body := videoRequestBody(t, req)
			for key, want := range tc.want {
				if got := body[key]; !reflect.DeepEqual(got, want) {
					t.Errorf("body[%q] = %#v, want %#v", key, got, want)
				}
			}
			for _, key := range tc.absent {
				if _, ok := body[key]; ok {
					t.Errorf("unset flag must not add body[%q] = %#v", key, body[key])
				}
			}
			if tc.jsonBody == "" {
				if !reflect.DeepEqual(body["vendor_obj"], map[string]any{"n": float64(1)}) {
					t.Errorf("vendor_obj must be preserved, got %#v", body["vendor_obj"])
				}
				if !reflect.DeepEqual(body["vendor_arr"], []any{float64(1), float64(2)}) {
					t.Errorf("vendor_arr must be preserved, got %#v", body["vendor_arr"])
				}
			}
		})
	}
}

func TestBuildVideoRequestJSONRefreshesTypedFields(t *testing.T) {
	cmd := videoTestCmd(t)
	options.Shared.JSONInput = `{"model":"json-model","prompt":"json-prompt","duration":8,` +
		`"size":"1:1","resolution":"480p","seed":7,"generate_audio":false,` +
		`"return_last_frame":false,"image_urls":["json.png"],"video_urls":["json.mp4"],` +
		`"audio_urls":["json.mp3"],"tools":[{"type":"json_tool"}],` +
		`"image_with_roles":[{"url":"json-first.png","role":"first_frame"}]}`

	setVideoFlag(t, "model", "flag-model")
	setVideoFlag(t, "prompt", "flag-prompt")
	setVideoFlag(t, "duration", "4")
	setVideoFlag(t, "seed", "11")
	setVideoFlag(t, "generate-audio", "true")
	setVideoFlag(t, "return-last-frame", "true")
	setVideoFlag(t, "image-url", "flag.png")
	setVideoFlag(t, "video-url", "flag.mp4")
	setVideoFlag(t, "audio-url", "flag.mp3")
	setVideoFlag(t, "tool", "web_search")
	setVideoFlag(t, "first-frame", "flag-first.png")

	req, err := buildVideoRequest(cmd)
	if err != nil {
		t.Fatalf("buildVideoRequest: %v", err)
	}

	if req.Model != "flag-model" {
		t.Errorf("Model = %q, want flag-model", req.Model)
	}
	if req.Prompt != "flag-prompt" {
		t.Errorf("Prompt = %q, want flag-prompt", req.Prompt)
	}
	if req.Duration == nil || *req.Duration != 4 {
		t.Errorf("Duration = %v, want 4", req.Duration)
	}
	if req.Seed == nil || *req.Seed != 11 {
		t.Errorf("Seed = %v, want 11", req.Seed)
	}
	if req.GenerateAudio == nil || !*req.GenerateAudio {
		t.Errorf("GenerateAudio = %v, want true", req.GenerateAudio)
	}
	if req.ReturnLastFrame == nil || !*req.ReturnLastFrame {
		t.Errorf("ReturnLastFrame = %v, want true", req.ReturnLastFrame)
	}
	for name, got := range map[string][]string{
		"ImageURLs": req.ImageURLs,
		"VideoURLs": req.VideoURLs,
		"AudioURLs": req.AudioURLs,
	} {
		var want []string
		switch name {
		case "ImageURLs":
			want = []string{"flag.png"}
		case "VideoURLs":
			want = []string{"flag.mp4"}
		default:
			want = []string{"flag.mp3"}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", name, got, want)
		}
	}
	if len(req.Tools) != 1 || req.Tools[0].Type != "web_search" {
		t.Errorf("Tools = %#v, want one web_search tool", req.Tools)
	}
	if len(req.ImageWithRoles) != 1 || req.ImageWithRoles[0].URL != "flag-first.png" || req.ImageWithRoles[0].Role != "first_frame" {
		t.Errorf("ImageWithRoles = %#v, want flag-first.png/first_frame", req.ImageWithRoles)
	}

	body := videoRequestBody(t, req)
	if body["model"] != "flag-model" || body["duration"] != float64(4) {
		t.Errorf("RawJSON must carry the overrides, got %s", req.RawJSON)
	}
}

func TestBuildVideoRequestJSONFromFileWithFlagOverride(t *testing.T) {
	cmd := videoTestCmd(t)
	path := filepath.Join(t.TempDir(), "request.json")
	raw := `{"model":"file-model","prompt":"file-prompt","vendor":true}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write JSON input: %v", err)
	}
	options.Shared.JSONInput = path
	setVideoFlag(t, "model", "flag-model")

	req, err := buildVideoRequest(cmd)
	if err != nil {
		t.Fatalf("buildVideoRequest: %v", err)
	}
	body := videoRequestBody(t, req)
	if body["model"] != "flag-model" {
		t.Errorf("model = %#v, want flag-model", body["model"])
	}
	if body["prompt"] != "file-prompt" || body["vendor"] != true {
		t.Errorf("untouched keys must survive the merge, got %s", req.RawJSON)
	}
}

func TestBuildVideoRequestJSONIgnoresBehavioralFlags(t *testing.T) {
	const jsonBody = `{"model":"m","prompt":"p"}`
	cmd := videoTestCmd(t)
	options.Shared.JSONInput = jsonBody

	for _, kv := range [][2]string{
		{"remix", "true"}, {"raw", "true"}, {"task-id", "task_1"}, {"job-id", "job_1"},
		{"dry-run", "true"}, {"preview", "true"}, {"gif", "true"}, {"gif-width", "240"},
		{"mp4", "true"}, {"crop-margin", "40"}, {"ffmpeg-flags", "-an"},
	} {
		setVideoFlag(t, kv[0], kv[1])
	}

	req, err := buildVideoRequest(cmd)
	if err != nil {
		t.Fatalf("buildVideoRequest: %v", err)
	}
	if string(req.RawJSON) != jsonBody {
		t.Errorf("behavioral flags must not touch the body:\n got %s\nwant %s", req.RawJSON, jsonBody)
	}

	body := videoRequestBody(t, req)
	for _, key := range []string{
		"remix", "raw", "task_id", "job_id", "dry_run", "preview", "gif",
		"gif_width", "mp4", "crop_margin", "ffmpeg_flags", "provider",
		"api_key", "api_base", "http_proxy", "output", "verbose", "timeout",
		"config", "print_config",
	} {
		if _, ok := body[key]; ok {
			t.Errorf("behavioral key %q must never be written to the body", key)
		}
	}
}
