package image

import "testing"

func TestImageNumberFlagSpellings(t *testing.T) {
	cmd := newJSONImageCmd(t)
	if f := cmd.Flags().ShorthandLookup("n"); f == nil || f.Name != "number" {
		t.Fatalf("-n shorthand = %v, want --number", f)
	}
	if cmd.Flags().Lookup("number") == nil || cmd.Flags().Lookup("n") == nil {
		t.Fatal("both --number and the legacy --n alias must exist")
	}

	for _, name := range []string{"number", "n"} {
		t.Run(name, func(t *testing.T) {
			cmd := newJSONImageCmd(t)
			for flag, value := range map[string]string{"prompt": "p", name: "2"} {
				if err := cmd.Flags().Set(flag, value); err != nil {
					t.Fatalf("set --%s: %v", flag, err)
				}
			}
			req, err := buildImageRequest(cmd)
			if err != nil {
				t.Fatalf("buildImageRequest() error = %v", err)
			}
			if req.N == nil || *req.N != 2 {
				t.Fatalf("req.N = %v, want 2 via --%s", req.N, name)
			}
		})
	}
}
