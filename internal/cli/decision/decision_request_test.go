package decision

import (
	"encoding/json"
	"fmt"
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

func TestLoadBank(t *testing.T) {
	origJSON, origCfg := options.Shared.JSONInput, options.Shared.Cfg
	defer func() { options.Shared.JSONInput, options.Shared.Cfg = origJSON, origCfg }()

	options.Shared.Cfg = nil
	options.Shared.JSONInput = `{"model":"tev1","state":"from bank","questions":{"intent":{"type":"choice","instructions":"which?","criteria":{"a":"A","b":"B"}}}}`
	b, err := loadBank()
	if err != nil {
		t.Fatalf("loadBank(inline) = %v, want nil", err)
	}
	if b.Model != "tev1" || string(b.State) != `"from bank"` || len(b.Questions) != 1 {
		t.Errorf("loadBank(inline) = %+v, want model tev1, bank state, 1 question", b)
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
