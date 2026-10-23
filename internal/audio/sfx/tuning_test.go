package sfx

import (
	"encoding/json"
	"reflect"
	"testing"
)

func ptrInt64(v int64) *int64 { return &v }
func ptrBool(v bool) *bool    { return &v }

func TestParseTuning(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    Tuning
		wantErr bool
	}{
		{name: "empty", raw: "", want: Tuning{}},
		{name: "steps", raw: `{"steps":12}`, want: Tuning{Steps: 12}},
		{name: "sigma", raw: `{"sigma_max":0.5}`, want: Tuning{SigmaMax: 0.5}},
		{name: "both", raw: `{"steps":6,"sigma_max":0.8}`, want: Tuning{Steps: 6, SigmaMax: 0.8}},
		{name: "seed", raw: `{"seed":42}`, want: Tuning{Seed: ptrInt64(42)}},
		{name: "normalize off", raw: `{"normalize":false}`, want: Tuning{Normalize: ptrBool(false)}},
		{name: "unknown key", raw: `{"cfg_scale":7}`, wantErr: true},
		{name: "steps too high", raw: `{"steps":99}`, wantErr: true},
		{name: "sigma too high", raw: `{"sigma_max":2}`, wantErr: true},
		{name: "sigma too low", raw: `{"sigma_max":0.001}`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseTuning(json.RawMessage(tc.raw))
			if tc.wantErr {
				if err == nil {
					t.Fatal("ParseTuning() = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTuning() error = %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseTuning() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
