package midjourney

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// ============================================================================
// Test command builders — a fresh command per test gives an isolated Changed()
// state, so one case cannot leak "flag was set" into the next. Flag
// registration also resets the package globals to their defaults, so callers
// must build the command before assigning those globals.
// ============================================================================

func newMJTestCmd(register ...func(*cobra.Command)) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	for _, r := range register {
		r(cmd)
	}
	return cmd
}

func newMJTaskActionTestCmd() *cobra.Command {
	return newMJTestCmd(func(c *cobra.Command) {
		registerTaskActionFlags(c)
		c.Flags().StringVar(&mjJSONInput, "json", "", "")
	})
}

func newMJImagineTestCmd() *cobra.Command {
	return newMJTestCmd(registerSharedFlags, registerImagineStructuredFlags)
}

func newMJZoomTestCmd() *cobra.Command {
	return newMJTestCmd(func(c *cobra.Command) {
		f := c.Flags()
		f.StringVar(&mjTaskID, "task-id", "", "")
		f.IntVar(&mjIndex, "index", 0, "")
		f.StringVar(&mjCustomID, "custom-id", "", "")
		f.Float64Var(&mjZoomRatio, "zoom-ratio", 0, "")
		f.StringVar(&mjSpeed, "speed", "", "")
		f.StringVar(&mjJSONInput, "json", "", "")
	})
}

func newMJRemixTestCmd() *cobra.Command {
	return newMJTestCmd(func(c *cobra.Command) {
		f := c.Flags()
		f.StringVar(&mjTaskID, "task-id", "", "")
		f.IntVar(&mjIndex, "index", 0, "")
		f.StringVarP(&mjPrompt, "prompt", "p", "", "")
		f.StringVar(&mjSpeed, "speed", "", "")
		f.StringVar(&mjJSONInput, "json", "", "")
	})
}

func setMJFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("set flag %s=%s: %v", name, value, err)
	}
}

// ============================================================================
// imagine: explicit flags override the JSON body, vendor keys survive
// ============================================================================

func TestBuildMJImagineReq_flagsOverrideJSON(t *testing.T) {
	const raw = `{"prompt":"old","size":"1:1","seed":1,"vendor_key":"keep","extra_body":{"image":"x"}}`
	tests := []struct {
		name        string
		flag        string
		value       string
		wantPrompt  string
		wantSize    string
		wantSeed    int
		wantHasSeed bool
	}{
		{name: "prompt", flag: "prompt", value: "new prompt", wantPrompt: "new prompt", wantSize: "1:1", wantSeed: 1, wantHasSeed: true},
		{name: "size", flag: "size", value: "16:9", wantPrompt: "old", wantSize: "16:9", wantSeed: 1, wantHasSeed: true},
		{name: "seed zero still overrides", flag: "seed", value: "0", wantPrompt: "old", wantSize: "1:1", wantSeed: 0, wantHasSeed: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newMJImagineTestCmd()
			mjJSONInput = raw
			setMJFlag(t, cmd, tc.flag, tc.value)

			req, err := buildMJImagineReq(cmd)
			if err != nil {
				t.Fatalf("buildMJImagineReq() error = %v", err)
			}
			if req.Prompt != tc.wantPrompt {
				t.Errorf("Prompt = %q, want %q", req.Prompt, tc.wantPrompt)
			}
			if req.Size != tc.wantSize {
				t.Errorf("Size = %q, want %q", req.Size, tc.wantSize)
			}
			if tc.wantHasSeed {
				if req.Seed == nil || *req.Seed != tc.wantSeed {
					t.Errorf("Seed = %v, want %d", req.Seed, tc.wantSeed)
				}
			}
			body := string(req.RawJSON)
			for _, want := range []string{`"vendor_key":"keep"`, `"extra_body":{"image":"x"}`} {
				if !strings.Contains(body, want) {
					t.Errorf("merged body lost vendor key: want %s in %s", want, body)
				}
			}
			if !strings.Contains(body, `"prompt":"`+tc.wantPrompt+`"`) {
				t.Errorf("merged body prompt = %s, want %q", body, tc.wantPrompt)
			}
		})
	}
}

func TestBuildMJImagineReq_jsonOnlyByteIdentity(t *testing.T) {
	const raw = `{"prompt":"p","vendor_key":"v","extra_body":{"image":"x"}}`
	cmd := newMJImagineTestCmd()
	mjJSONInput = raw

	req, err := buildMJImagineReq(cmd)
	if err != nil {
		t.Fatalf("buildMJImagineReq() error = %v", err)
	}
	if string(req.RawJSON) != raw {
		t.Errorf("RawJSON = %s, want byte-identical to %s", req.RawJSON, raw)
	}
}

// ============================================================================
// task action: --index 0 is written (previously dropped by mjIndex > 0)
// ============================================================================

func TestBuildMJTaskActionReqFromJSON_indexOverlay(t *testing.T) {
	tests := []struct {
		name     string
		jsonRaw  string
		setIndex *string
		wantIdx  int
	}{
		{
			name:     "no flag keeps JSON index",
			jsonRaw:  `{"task_id":"task_json","index":3,"speed":"turbo"}`,
			setIndex: nil,
			wantIdx:  3,
		},
		{
			name:     "explicit zero overrides",
			jsonRaw:  `{"task_id":"task_json","index":3}`,
			setIndex: strPtr("0"),
			wantIdx:  0,
		},
		{
			name:     "explicit two overrides",
			jsonRaw:  `{"task_id":"task_json","index":1}`,
			setIndex: strPtr("2"),
			wantIdx:  2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newMJTaskActionTestCmd()
			mjJSONInput = tc.jsonRaw
			if tc.setIndex != nil {
				setMJFlag(t, cmd, "index", *tc.setIndex)
			}

			req, err := buildMJTaskActionReqFromJSON(cmd)
			if err != nil {
				t.Fatalf("buildMJTaskActionReqFromJSON() error = %v", err)
			}
			if req.Index == nil || *req.Index != tc.wantIdx {
				t.Errorf("Index = %v, want %d", req.Index, tc.wantIdx)
			}
			if body := string(req.RawJSON); !strings.Contains(body, `"index":`+strconv.Itoa(tc.wantIdx)) {
				t.Errorf("merged body = %s, want index %d", body, tc.wantIdx)
			}
		})
	}
}

// ============================================================================
// zoom: --zoom-ratio overrides the JSON body
// ============================================================================

func TestBuildMJZoomReq_zoomRatioOverlay(t *testing.T) {
	cmd := newMJZoomTestCmd()
	mjJSONInput = `{"task_id":"t","zoom_ratio":1.5,"custom_key":"keep"}`
	setMJFlag(t, cmd, "zoom-ratio", "2.5")

	req, err := buildMJZoomReq(cmd)
	if err != nil {
		t.Fatalf("buildMJZoomReq() error = %v", err)
	}
	if req.ZoomRatio == nil || *req.ZoomRatio != 2.5 {
		t.Errorf("ZoomRatio = %v, want 2.5", req.ZoomRatio)
	}
	body := string(req.RawJSON)
	if !strings.Contains(body, `"zoom_ratio":2.5`) {
		t.Errorf("merged body = %s, want zoom_ratio 2.5", body)
	}
	if !strings.Contains(body, `"custom_key":"keep"`) {
		t.Errorf("merged body lost custom key: %s", body)
	}
}

// ============================================================================
// remix: --index 0 is written, --prompt overrides, no-flags is byte-identical
// ============================================================================

func TestBuildMJRemixReq_overlay(t *testing.T) {
	cmd := newMJRemixTestCmd()
	mjJSONInput = `{"task_id":"t","prompt":"old","vendor_key":"keep"}`
	setMJFlag(t, cmd, "prompt", "new")
	setMJFlag(t, cmd, "index", "0")

	req, err := buildMJRemixReq(cmd)
	if err != nil {
		t.Fatalf("buildMJRemixReq() error = %v", err)
	}
	if req.Prompt != "new" {
		t.Errorf("Prompt = %q, want new", req.Prompt)
	}
	if req.Index == nil || *req.Index != 0 {
		t.Errorf("Index = %v, want 0", req.Index)
	}
	body := string(req.RawJSON)
	if !strings.Contains(body, `"index":0`) {
		t.Errorf("merged body = %s, want index 0", body)
	}
	if !strings.Contains(body, `"prompt":"new"`) {
		t.Errorf("merged body = %s, want prompt new", body)
	}
	if !strings.Contains(body, `"vendor_key":"keep"`) {
		t.Errorf("merged body lost vendor key: %s", body)
	}
}

func TestBuildMJRemixReq_jsonOnlyByteIdentity(t *testing.T) {
	const raw = `{"task_id":"t","prompt":"p"}`
	cmd := newMJRemixTestCmd()
	mjJSONInput = raw

	req, err := buildMJRemixReq(cmd)
	if err != nil {
		t.Fatalf("buildMJRemixReq() error = %v", err)
	}
	if string(req.RawJSON) != raw {
		t.Errorf("RawJSON = %s, want byte-identical to %s", req.RawJSON, raw)
	}
}

func strPtr(s string) *string { return &s }

// ============================================================================
// MJRemixRequest.Index json tag: omitted when nil, present for explicit zero
// ============================================================================

func TestMJRemixRequest_indexOmitEmpty(t *testing.T) {
	req := types.MJRemixRequest{TaskID: "task_v8"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if strings.Contains(string(data), `"index"`) {
		t.Errorf("nil Index must be omitted, got %s", data)
	}
}

func TestMJRemixRequest_explicitZeroIndexSerialized(t *testing.T) {
	idx := 0
	req := types.MJRemixRequest{TaskID: "task_v8", Index: &idx}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), `"index":0`) {
		t.Errorf("explicit zero Index must be serialized, got %s", data)
	}
}
