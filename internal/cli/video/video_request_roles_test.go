package video

import (
	"encoding/json"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

func TestBuildVideoRequestJSONImageWithRoles(t *testing.T) {
	tests := []struct {
		name     string
		jsonBody string
		set      func(t *testing.T)
		want     []map[string]string
	}{
		{
			name:     "first and last frame from flags",
			jsonBody: `{"model":"m"}`,
			set: func(t *testing.T) {
				setVideoFlag(t, "first-frame", "day.png")
				setVideoFlag(t, "last-frame", "night.png")
			},
			want: []map[string]string{
				{"url": "day.png", "role": "first_frame"},
				{"url": "night.png", "role": "last_frame"},
			},
		},
		{
			name:     "preserves existing other-role entries",
			jsonBody: `{"model":"m","image_with_roles":[{"url":"ref.png","role":"reference_image"}]}`,
			set:      func(t *testing.T) { setVideoFlag(t, "first-frame", "day.png") },
			want: []map[string]string{
				{"url": "ref.png", "role": "reference_image"},
				{"url": "day.png", "role": "first_frame"},
			},
		},
		{
			name:     "overrides an existing first_frame entry in place",
			jsonBody: `{"model":"m","image_with_roles":[{"url":"old.png","role":"first_frame"},{"url":"ref.png","role":"reference_image"}]}`,
			set:      func(t *testing.T) { setVideoFlag(t, "first-frame", "new.png") },
			want: []map[string]string{
				{"url": "new.png", "role": "first_frame"},
				{"url": "ref.png", "role": "reference_image"},
			},
		},
		{
			name:     "empty first-frame drops the existing entry",
			jsonBody: `{"model":"m","image_with_roles":[{"url":"old.png","role":"first_frame"}]}`,
			set:      func(t *testing.T) { setVideoFlag(t, "first-frame", "") },
			want:     []map[string]string{},
		},
		{
			name:     "last-frame alone keeps an existing first_frame",
			jsonBody: `{"model":"m","image_with_roles":[{"url":"keep.png","role":"first_frame"}]}`,
			set:      func(t *testing.T) { setVideoFlag(t, "last-frame", "night.png") },
			want: []map[string]string{
				{"url": "keep.png", "role": "first_frame"},
				{"url": "night.png", "role": "last_frame"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := videoTestCmd(t)
			options.Shared.JSONInput = tc.jsonBody
			tc.set(t)

			req, err := buildVideoRequest(cmd)
			if err != nil {
				t.Fatalf("buildVideoRequest: %v", err)
			}

			var body struct {
				ImageWithRoles []map[string]string `json:"image_with_roles"`
			}
			if err := json.Unmarshal(req.RawJSON, &body); err != nil {
				t.Fatalf("unmarshal merged body: %v", err)
			}
			assertImageWithRoles(t, body.ImageWithRoles, tc.want)

			typed := make([]map[string]string, 0, len(req.ImageWithRoles))
			for _, role := range req.ImageWithRoles {
				typed = append(typed, map[string]string{"url": role.URL, "role": role.Role})
			}
			assertImageWithRoles(t, typed, tc.want)
		})
	}
}

func assertImageWithRoles(t *testing.T, got, want []map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("image_with_roles = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i]["url"] != want[i]["url"] || got[i]["role"] != want[i]["role"] {
			t.Errorf("image_with_roles[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
