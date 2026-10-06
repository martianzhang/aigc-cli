package decision

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func questionBank(n int) map[string]json.RawMessage {
	questions := make(map[string]json.RawMessage, n)
	for i := 0; i < n; i++ {
		questions[fmt.Sprintf("q%02d", i)] = json.RawMessage(`{"type":"noul","instructions":"ok?"}`)
	}
	return questions
}

func TestFilterQuestions(t *testing.T) {
	questions := questionBank(3)

	got, err := filterQuestions(questions, nil)
	if err != nil {
		t.Fatalf("filterQuestions(all) = %v, want nil", err)
	}
	if len(got) != 3 {
		t.Errorf("filterQuestions(all) selected %d, want 3", len(got))
	}

	got, err = filterQuestions(questions, []string{"q01", "q02"})
	if err != nil {
		t.Fatalf("filterQuestions(subset) = %v, want nil", err)
	}
	if len(got) != 2 {
		t.Fatalf("filterQuestions(subset) selected %d, want 2", len(got))
	}
	for name := range got {
		if name != "q01" && name != "q02" {
			t.Errorf("filterQuestions(subset) selected %q, want only q01/q02", name)
		}
	}

	got, err = filterQuestions(questions, []string{" q01 ", "", "q01"})
	if err != nil {
		t.Fatalf("filterQuestions(trim/dedupe) = %v, want nil", err)
	}
	if len(got) != 1 {
		t.Errorf("filterQuestions(trim/dedupe) selected %d, want 1", len(got))
	}

	if _, err := filterQuestions(questions, []string{"nope"}); err == nil {
		t.Error("filterQuestions(unknown) = nil error, want error listing valid names")
	} else if !strings.Contains(err.Error(), "q00") || !strings.Contains(err.Error(), "unknown question") {
		t.Errorf("filterQuestions(unknown) = %v, want error listing valid names", err)
	}

	if _, err := filterQuestions(questions, []string{"", " "}); err == nil {
		t.Error("filterQuestions(empty selection) = nil error, want error")
	}

	if _, err := filterQuestions(questionBank(65), nil); err == nil {
		t.Error("filterQuestions(65 in bank, no filter) = nil error, want >64 error")
	}

	names := make([]string, 65)
	for i := range names {
		names[i] = fmt.Sprintf("q%02d", i)
	}
	big := questionBank(65)
	if _, err := filterQuestions(big, names); err == nil {
		t.Error("filterQuestions(65 selected) = nil error, want >64 error")
	}
}

func TestResolveState(t *testing.T) {
	tests := []struct {
		name      string
		flagValue string
		bankState json.RawMessage
		want      string
		wantErr   bool
	}{
		{name: "plain text becomes a JSON string", flagValue: "hello world", want: `"hello world"`},
		{name: "JSON object passes through raw", flagValue: `{"policy":"Refunds within 30 days."}`, want: `{"policy":"Refunds within 30 days."}`},
		{name: "JSON array passes through raw", flagValue: `["a","b"]`, want: `["a","b"]`},
		{name: "object-like invalid JSON becomes a string", flagValue: "{oops", want: `"{oops"`},
		{name: "quoted JSON scalar becomes a string", flagValue: `"already quoted"`, want: `"\"already quoted\""`},
		{name: "bank state used when flag unset", bankState: json.RawMessage(`"from bank"`), want: `"from bank"`},
		{name: "bank object state used when flag unset", bankState: json.RawMessage(`{"k":1}`), want: `{"k":1}`},
		{name: "missing state errors", wantErr: true},
		{name: "whitespace-only state errors", flagValue: "   ", wantErr: true},
	}
	for _, tc := range tests {
		got, err := resolveState(tc.flagValue, tc.bankState)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: resolveState() = nil error, want error", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: resolveState() = %v, want nil", tc.name, err)
		}
		if string(got) != tc.want {
			t.Errorf("%s: resolveState() = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestResolveModel(t *testing.T) {
	tests := []struct {
		name                string
		cli, bank, ep, want string
	}{
		{"cli wins", "cli-m", "bank-m", "ep-m", "cli-m"},
		{"bank beats config", "", "bank-m", "ep-m", "bank-m"},
		{"config model used", "", "", "ep-m", "ep-m"},
		{"code default", "", "", "", "tev1"},
	}
	for _, tc := range tests {
		if got := resolveModel(tc.cli, tc.bank, tc.ep); got != tc.want {
			t.Errorf("%s: resolveModel(%q,%q,%q) = %q, want %q", tc.name, tc.cli, tc.bank, tc.ep, got, tc.want)
		}
	}
}

func TestResolveImages(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	pngBytes := buf.Bytes()
	pngPath := filepath.Join(dir, "form.png")
	if err := os.WriteFile(pngPath, pngBytes, 0o644); err != nil {
		t.Fatalf("write png fixture: %v", err)
	}
	rawB64 := base64.StdEncoding.EncodeToString(pngBytes)
	dataURI := "data:image/png;base64," + rawB64
	otherURI := "data:image/png;base64,Zm9v"

	t.Run("nil and empty inputs", func(t *testing.T) {
		got, err := resolveImages(nil, nil, 0)
		if err != nil || got != nil {
			t.Errorf("resolveImages(nil,nil) = (%v,%v), want (nil,nil)", got, err)
		}
		got, err = resolveImages([]string{}, []string{}, 0)
		if err != nil || got != nil {
			t.Errorf("resolveImages(empty,empty) = (%v,%v), want (nil,nil)", got, err)
		}
	})

	t.Run("data URI stripped to raw base64", func(t *testing.T) {
		got, err := resolveImages([]string{dataURI}, nil, 0)
		if err != nil {
			t.Fatalf("resolveImages(dataURI) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != rawB64 {
			t.Errorf("resolveImages(dataURI) = %v, want [%s]", got, rawB64)
		}
	})

	t.Run("local file encoded to raw base64", func(t *testing.T) {
		got, err := resolveImages([]string{pngPath}, nil, 0)
		if err != nil {
			t.Fatalf("resolveImages(file) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != rawB64 {
			t.Errorf("resolveImages(file) = %v, want raw base64 of file bytes", got)
		}
	})

	t.Run("blank entries skipped", func(t *testing.T) {
		got, err := resolveImages([]string{"", "   ", dataURI}, nil, 0)
		if err != nil {
			t.Fatalf("resolveImages(blank) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != rawB64 {
			t.Errorf("resolveImages(blank) = %v, want 1 image", got)
		}
	})

	t.Run("non-file non-dataURI rejected", func(t *testing.T) {
		_, err := resolveImages([]string{"https://example.com/form.png"}, nil, 0)
		if err == nil || !strings.Contains(err.Error(), "does not accept URLs") {
			t.Errorf("resolveImages(url) error = %v, want URL rejection", err)
		}
	})

	t.Run("CLI overrides bank", func(t *testing.T) {
		got, err := resolveImages([]string{dataURI}, []string{otherURI}, 0)
		if err != nil {
			t.Fatalf("resolveImages(cli+bank) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != rawB64 {
			t.Errorf("resolveImages(cli+bank) = %v, want only the CLI image %s", got, rawB64)
		}
	})

	t.Run("bank used when CLI empty", func(t *testing.T) {
		got, err := resolveImages(nil, []string{pngPath}, 0)
		if err != nil {
			t.Fatalf("resolveImages(bank) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != rawB64 {
			t.Errorf("resolveImages(bank) = %v, want the bank image", got)
		}
	})

	t.Run("maxEdge downscales before encoding", func(t *testing.T) {
		var big bytes.Buffer
		if err := png.Encode(&big, image.NewRGBA(image.Rect(0, 0, 200, 100))); err != nil {
			t.Fatalf("encode big png: %v", err)
		}
		bigPath := filepath.Join(dir, "big.png")
		if err := os.WriteFile(bigPath, big.Bytes(), 0o644); err != nil {
			t.Fatalf("write big png: %v", err)
		}
		got, err := resolveImages([]string{bigPath}, nil, 50)
		if err != nil {
			t.Fatalf("resolveImages(resize) = %v, want nil", err)
		}
		raw, err := base64.StdEncoding.DecodeString(got[0])
		if err != nil {
			t.Fatalf("decode resized base64: %v", err)
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("decode resized image: %v", err)
		}
		if cfg.Width != 50 || cfg.Height != 25 {
			t.Errorf("resized dims = %dx%d, want 50x25", cfg.Width, cfg.Height)
		}
	})

	t.Run("maxEdge 0 keeps original bytes", func(t *testing.T) {
		got, err := resolveImages([]string{pngPath}, nil, 0)
		if err != nil {
			t.Fatalf("resolveImages(no resize) = %v, want nil", err)
		}
		if len(got) != 1 || got[0] != rawB64 {
			t.Errorf("resolveImages(no resize) = %v, want unchanged %s", got, rawB64)
		}
	})
}

func TestStripDataURIBase64(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		want    string
		wantErr string
	}{
		{name: "valid data URI", uri: "data:image/png;base64,aGVsbG8=", want: "aGVsbG8="},
		{name: "missing comma", uri: "data:image/png;base64", wantErr: "missing comma"},
		{name: "non-base64 data URI", uri: "data:image/png;hex,0a0b", wantErr: "must be base64-encoded"},
		{name: "empty payload", uri: "data:image/png;base64,", wantErr: "contains no data"},
	}
	for _, tc := range tests {
		got, err := stripDataURIBase64(tc.uri)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%s: stripDataURIBase64(%q) error = %v, want %q", tc.name, tc.uri, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: stripDataURIBase64(%q) = %v, want nil", tc.name, tc.uri, err)
		}
		if got != tc.want {
			t.Errorf("%s: stripDataURIBase64(%q) = %q, want %q", tc.name, tc.uri, got, tc.want)
		}
	}
}

func TestLoadBank(t *testing.T) {
	origJSON, origCfg := options.Shared.JSONInput, options.Shared.Cfg
	defer func() { options.Shared.JSONInput, options.Shared.Cfg = origJSON, origCfg }()

	options.Shared.Cfg = nil
	options.Shared.JSONInput = `{"model":"tev1","state":"from bank","images":["a.png"],"questions":{"intent":{"type":"choice","instructions":"which?","criteria":{"a":"A","b":"B"}}}}`
	b, err := loadBank()
	if err != nil {
		t.Fatalf("loadBank(inline) = %v, want nil", err)
	}
	if b.Model != "tev1" || string(b.State) != `"from bank"` || len(b.Questions) != 1 {
		t.Errorf("loadBank(inline) = %+v, want model tev1, bank state, 1 question", b)
	}
	if len(b.Images) != 1 || b.Images[0] != "a.png" {
		t.Errorf("loadBank(inline) images = %v, want [a.png]", b.Images)
	}

	options.Shared.JSONInput = `{
		// JSONC: comments and trailing commas are allowed
		"questions": {"refund": {"type": "noul", "instructions": "refund?",},},
	}`
	if b, err = loadBank(); err != nil {
		t.Fatalf("loadBank(JSONC) = %v, want nil", err)
	} else if len(b.Questions) != 1 {
		t.Errorf("loadBank(JSONC) parsed %d questions, want 1", len(b.Questions))
	}

	options.Shared.JSONInput = ""
	options.Shared.Cfg = &types.Config{Defaults: &types.ConfigDefaults{
		Decision: &types.DecisionDefaults{Bank: `{"questions":{"a":{"type":"noul","instructions":"?"}}}`},
	}}
	if _, err = loadBank(); err != nil {
		t.Fatalf("loadBank(config bank) = %v, want nil", err)
	}

	options.Shared.Cfg = nil
	if _, err = loadBank(); err == nil {
		t.Error("loadBank(no source) = nil error, want error")
	}

	options.Shared.JSONInput = `{"questions":{}}`
	if _, err = loadBank(); err == nil {
		t.Error("loadBank(zero questions) = nil error, want error")
	}
}
