package image

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestSplitSizeTier(t *testing.T) {
	tests := []struct {
		name      string
		in        string
		wantSize  string
		wantRatio string
		wantErr   bool
	}{
		{name: "plain ratio", in: "16:9", wantSize: "16:9"},
		{name: "pixel dims", in: "1024x1024", wantSize: "1024x1024"},
		{name: "bare tier", in: "2K", wantSize: "2K"},
		{name: "compound", in: "2K@16:9", wantSize: "2K", wantRatio: "16:9"},
		{name: "compound padded", in: " 2K@16:9 ", wantSize: "2K", wantRatio: "16:9"},
		{name: "missing tier", in: "@16:9", wantErr: true},
		{name: "missing ratio", in: "2K@", wantErr: true},
		{name: "second at sign", in: "2K@16:9@x", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			size, ratio, err := splitSizeTier(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("splitSizeTier(%q) error = nil, want error", tc.in)
				}
				if !strings.Contains(err.Error(), `invalid --size`) {
					t.Errorf("error = %v, want invalid --size error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitSizeTier(%q) error = %v", tc.in, err)
			}
			if size != tc.wantSize || ratio != tc.wantRatio {
				t.Errorf("splitSizeTier(%q) = (%q, %q), want (%q, %q)",
					tc.in, size, ratio, tc.wantSize, tc.wantRatio)
			}
		})
	}
}

func TestNormalizeSizeTier(t *testing.T) {
	t.Run("no at sign is a no-op", func(t *testing.T) {
		raw := json.RawMessage(`{"model":"m","prompt":"p","size":"1024x1024","seed":1}`)
		req := &types.GenerateRequest{Size: "1024x1024", RawJSON: append(json.RawMessage{}, raw...)}
		if err := normalizeSizeTier(req); err != nil {
			t.Fatalf("normalizeSizeTier() error = %v", err)
		}
		if req.Size != "1024x1024" || req.Ratio != "" {
			t.Errorf("typed fields changed: size=%q ratio=%q", req.Size, req.Ratio)
		}
		if !bytes.Equal(req.RawJSON, raw) {
			t.Errorf("RawJSON changed:\n got: %s\nwant: %s", req.RawJSON, raw)
		}
	})

	t.Run("typed split", func(t *testing.T) {
		req := &types.GenerateRequest{Size: "2K@16:9"}
		if err := normalizeSizeTier(req); err != nil {
			t.Fatalf("normalizeSizeTier() error = %v", err)
		}
		if req.Size != "2K" || req.Ratio != "16:9" {
			t.Errorf("size/ratio = (%q, %q), want (2K, 16:9)", req.Size, req.Ratio)
		}
	})

	t.Run("conflict with existing ratio", func(t *testing.T) {
		req := &types.GenerateRequest{Size: "2K@16:9", Ratio: "4:3"}
		err := normalizeSizeTier(req)
		if err == nil || !strings.Contains(err.Error(), "conflicting ratio") {
			t.Fatalf("error = %v, want conflicting ratio error", err)
		}
		if req.Size != "2K@16:9" || req.Ratio != "4:3" {
			t.Errorf("conflict must not mutate fields, got size=%q ratio=%q", req.Size, req.Ratio)
		}
	})

	t.Run("equal ratio is accepted", func(t *testing.T) {
		req := &types.GenerateRequest{Size: "2K@16:9", Ratio: "16:9"}
		if err := normalizeSizeTier(req); err != nil {
			t.Fatalf("normalizeSizeTier() error = %v", err)
		}
		if req.Size != "2K" || req.Ratio != "16:9" {
			t.Errorf("size/ratio = (%q, %q), want (2K, 16:9)", req.Size, req.Ratio)
		}
	})

	t.Run("raw json body split", func(t *testing.T) {
		req := &types.GenerateRequest{
			Size:    "2K@16:9",
			RawJSON: json.RawMessage(`{"model":"m","prompt":"p","size":"2K@16:9","seed":1}`),
		}
		if err := normalizeSizeTier(req); err != nil {
			t.Fatalf("normalizeSizeTier() error = %v", err)
		}
		body := decodeRawBody(t, req.RawJSON)
		if body["size"] != "2K" {
			t.Errorf("body size = %#v, want 2K", body["size"])
		}
		if body["ratio"] != "16:9" {
			t.Errorf("body ratio = %#v, want 16:9", body["ratio"])
		}
		// Unrelated keys survive the round trip.
		if body["model"] != "m" || body["prompt"] != "p" || body["seed"] != json.Number("1") {
			t.Errorf("unrelated keys lost: %v", body)
		}
	})

	t.Run("raw json ratio conflict", func(t *testing.T) {
		req := &types.GenerateRequest{
			Size:    "2K@16:9",
			RawJSON: json.RawMessage(`{"prompt":"p","size":"2K@16:9","ratio":"4:3"}`),
		}
		err := normalizeSizeTier(req)
		if err == nil || !strings.Contains(err.Error(), "conflicting ratio in JSON body") {
			t.Fatalf("error = %v, want raw body conflicting ratio error", err)
		}
	})

	t.Run("raw json without compound size stays untouched", func(t *testing.T) {
		raw := json.RawMessage(`{"prompt":"p","size":"1024x1024"}`)
		req := &types.GenerateRequest{Size: "2K@16:9", RawJSON: append(json.RawMessage{}, raw...)}
		if err := normalizeSizeTier(req); err != nil {
			t.Fatalf("normalizeSizeTier() error = %v", err)
		}
		if req.Size != "2K" || req.Ratio != "16:9" {
			t.Errorf("typed split missing: size=%q ratio=%q", req.Size, req.Ratio)
		}
		if !bytes.Equal(req.RawJSON, raw) {
			t.Errorf("RawJSON changed:\n got: %s\nwant: %s", req.RawJSON, raw)
		}
	})

	t.Run("error paths", func(t *testing.T) {
		tests := []struct {
			name    string
			req     *types.GenerateRequest
			wantErr string
		}{
			{"invalid compound size", &types.GenerateRequest{Size: "2K@"}, "invalid --size"},
			{"raw json decode error", &types.GenerateRequest{Size: "2K@16:9", RawJSON: json.RawMessage(`{"size":`)}, "failed to parse JSON body"},
			{"raw json invalid compound size", &types.GenerateRequest{Size: "2K@16:9", RawJSON: json.RawMessage(`{"size":"2K@"}`)}, "invalid --size"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := normalizeSizeTier(tc.req)
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want %q", err, tc.wantErr)
				}
			})
		}
	})
}

// TestBuildImagePlanSizeTierNormalization pins the compound "<tier>@<ratio>"
// size split at the plan level: typed fields become size + aspect_ratio on
// OpenRouter, a verbatim --json body is split in place, and plain sizes stay
// untouched.
func TestBuildImagePlanSizeTierNormalization(t *testing.T) {
	p := &provider.EffectiveProvider{BaseURL: "https://openrouter.ai/api/v1", APIKey: "k", ProviderType: provider.OpenRouter}

	t.Run("typed compound size splits into size and aspect_ratio", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p", Size: "2K@16:9"}
		pl := mustBuildPlan(t, req, p)
		body := planBodyJSON(t, pl.Body)
		for _, frag := range []string{`"size":"2K"`, `"aspect_ratio":"16:9"`} {
			if !strings.Contains(body, frag) {
				t.Errorf("body missing %s, got:\n%s", frag, body)
			}
		}
		if strings.Contains(body, "@") {
			t.Errorf("body should not carry the compound value, got:\n%s", body)
		}
	})

	t.Run("verbatim json body size is split in place", func(t *testing.T) {
		req := &types.GenerateRequest{
			Size:    "2K@16:9",
			RawJSON: json.RawMessage(`{"model":"m","prompt":"p","size":"2K@16:9","seed":1}`),
		}
		pl := mustBuildPlan(t, req, p)
		body := planBodyJSON(t, pl.Body)
		for _, frag := range []string{`"size":"2K"`, `"ratio":"16:9"`, `"seed":1`} {
			if !strings.Contains(body, frag) {
				t.Errorf("body missing %s, got:\n%s", frag, body)
			}
		}
	})

	t.Run("plain size stays unchanged", func(t *testing.T) {
		req := &types.GenerateRequest{Model: "m", Prompt: "p", Size: "16:9"}
		pl := mustBuildPlan(t, req, p)
		body := planBodyJSON(t, pl.Body)
		if !strings.Contains(body, `"size":"16:9"`) {
			t.Errorf("body missing plain size, got:\n%s", body)
		}
		if strings.Contains(body, "aspect_ratio") {
			t.Errorf("plain size must not emit aspect_ratio, got:\n%s", body)
		}
	})
}
