package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeJSONOverlay(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		set           map[string]any
		want          string
		contain       []string
		notContain    []string
		wantIdentical bool
		wantErr       bool
	}{
		{
			name:          "empty set returns input unchanged",
			raw:           `{"model":"m","seed":1234567890123456789}`,
			set:           nil,
			wantIdentical: true,
		},
		{
			name:    "vendor keys preserved",
			raw:     `{"model":"krea/Krea-2-Turbo","loras":{"a":1.0},"steps":8}`,
			set:     map[string]any{"prompt": "x"},
			want:    `{"loras":{"a":1.0},"model":"krea/Krea-2-Turbo","prompt":"x","steps":8}`,
			contain: []string{`"loras":{"a":1.0}`, `"steps":8`},
		},
		{
			name:    "number fidelity via UseNumber",
			raw:     `{"seed":1234567890123456789,"guidance":1.0}`,
			set:     map[string]any{"prompt": "x"},
			want:    `{"guidance":1.0,"prompt":"x","seed":1234567890123456789}`,
			contain: []string{"1234567890123456789", "1.0"},
		},
		{
			name: "explicit zero and empty values written",
			raw:  `{"prompt":"x"}`,
			set:  map[string]any{"n": 0, "style": ""},
			want: `{"n":0,"prompt":"x","style":""}`,
		},
		{
			name: "dotted key creates nested object",
			raw:  `{"model":"m"}`,
			set:  map[string]any{"extra_body.image": "x"},
			want: `{"extra_body":{"image":"x"},"model":"m"}`,
		},
		{
			name:       "html characters not escaped",
			raw:        `{}`,
			set:        map[string]any{"prompt": "a<b>&c"},
			want:       `{"prompt":"a<b>&c"}`,
			notContain: []string{`\u003c`, `\u003e`, `\u0026`},
		},
		{
			name: "existing key overwritten",
			raw:  `{"prompt":"old"}`,
			set:  map[string]any{"prompt": "new"},
			want: `{"prompt":"new"}`,
		},
		{
			name: "nested merge preserves siblings",
			raw:  `{"extra_body":{"seed":1}}`,
			set:  map[string]any{"extra_body.image": "x"},
			want: `{"extra_body":{"image":"x","seed":1}}`,
		},
		{
			name: "deep dotted path",
			raw:  `{}`,
			set:  map[string]any{"a.b.c": true},
			want: `{"a":{"b":{"c":true}}}`,
		},
		{
			name:    "array raw rejected",
			raw:     `[1,2]`,
			set:     map[string]any{"a": 1},
			wantErr: true,
		},
		{
			name:    "scalar raw rejected",
			raw:     `42`,
			set:     map[string]any{"a": 1},
			wantErr: true,
		},
		{
			name:    "empty raw rejected",
			raw:     ``,
			set:     map[string]any{"a": 1},
			wantErr: true,
		},
		{
			name:    "null raw rejected",
			raw:     `null`,
			set:     map[string]any{"a": 1},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MergeJSONOverlay(json.RawMessage(tc.raw), tc.set)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("MergeJSONOverlay() error = nil, want non-nil (out=%s)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("MergeJSONOverlay() error = %v", err)
			}
			if tc.wantIdentical && !bytes.Equal(got, []byte(tc.raw)) {
				t.Fatalf("MergeJSONOverlay() = %q, want byte-identical to %q", got, tc.raw)
			}
			if tc.want != "" && string(got) != tc.want {
				t.Errorf("MergeJSONOverlay() = %s, want %s", got, tc.want)
			}
			for _, sub := range tc.contain {
				if !strings.Contains(string(got), sub) {
					t.Errorf("MergeJSONOverlay() = %s, want it to contain %q", got, sub)
				}
			}
			for _, sub := range tc.notContain {
				if strings.Contains(string(got), sub) {
					t.Errorf("MergeJSONOverlay() = %s, want it NOT to contain %q", got, sub)
				}
			}
		})
	}
}

func TestMergeJSONOverlayDoesNotMutateSetValues(t *testing.T) {
	shared := map[string]any{"image": "x"}
	set := map[string]any{"extra_body": shared}

	if _, err := MergeJSONOverlay(json.RawMessage(`{}`), set); err != nil {
		t.Fatalf("first call error = %v", err)
	}
	set2 := map[string]any{"a.b": 1}
	if _, err := MergeJSONOverlay(json.RawMessage(`{}`), set2); err != nil {
		t.Fatalf("second call error = %v", err)
	}
	if len(shared) != 1 || shared["image"] != "x" {
		t.Errorf("set value mutated: %#v", shared)
	}
}
