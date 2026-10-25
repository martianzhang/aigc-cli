package chat

import (
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

func TestDefaultConfirmPolicy(t *testing.T) {
	p, err := newConfirmPolicy(nil)
	if err != nil {
		t.Fatalf("newConfirmPolicy(nil) error = %v", err)
	}
	for _, tool := range []string{"bash", "write", "edit"} {
		if !p.needs(tool, `{"command":"ls","filepath":"a.go","old_string":"a","new_string":"b"}`) {
			t.Errorf("default policy should require confirmation for %s", tool)
		}
	}
	if p.needs("read", `{"filepath":"a.go"}`) {
		t.Error("default policy should not require confirmation for read")
	}
	if (*confirmPolicy)(nil).needs("bash", `{"command":"rm -rf /"}`) {
		t.Error("nil policy should never require confirmation")
	}
}

func TestConfirmPolicyEmptyDisables(t *testing.T) {
	p, err := newConfirmPolicy(&types.ChatDefaults{ConfirmTools: []string{}})
	if err != nil {
		t.Fatalf("newConfirmPolicy error = %v", err)
	}
	if p.needs("bash", `{"command":"rm -rf /"}`) {
		t.Error("explicit empty confirm_tools should disable confirmation")
	}
}

func TestConfirmPolicyCustomRegex(t *testing.T) {
	p, err := newConfirmPolicy(&types.ChatDefaults{
		ConfirmTools: []string{`^bash .*rm\s+-rf`, `^(write|edit) .*\.env$`},
	})
	if err != nil {
		t.Fatalf("newConfirmPolicy error = %v", err)
	}

	tests := []struct {
		name string
		tool string
		args string
		want bool
	}{
		{"rm -rf", "bash", `{"command":"rm -rf /tmp/x"}`, true},
		{"harmless ls", "bash", `{"command":"ls -la"}`, false},
		{"write .env", "write", `{"filepath":"/app/.env","content":"X=1"}`, true},
		{"edit .env", "edit", `{"filepath":".env","old_string":"a","new_string":"b"}`, true},
		{"write normal file", "write", `{"filepath":"main.go","content":"x"}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := p.needs(tt.tool, tt.args); got != tt.want {
				t.Errorf("needs(%s) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func TestConfirmPolicyInvalidRegex(t *testing.T) {
	_, err := newConfirmPolicy(&types.ChatDefaults{ConfirmTools: []string{`^bash(`}})
	if err == nil {
		t.Fatal("expected an error for an invalid regex")
	}
	if !strings.Contains(err.Error(), "confirm_tools") {
		t.Errorf("error should mention the config key, got %v", err)
	}
}

func TestConfirmPolicyAlwaysAllow(t *testing.T) {
	p, err := newConfirmPolicy(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.needs("bash", `{"command":"ls"}`) {
		t.Fatal("bash should need confirmation before allowAlways")
	}
	p.allowAlways("bash")
	if p.needs("bash", `{"command":"rm -rf /"}`) {
		t.Error("bash should be skipped after allowAlways")
	}
	if !p.needs("write", `{"filepath":"a.go","content":"x"}`) {
		t.Error("allowAlways(bash) must not affect other tools")
	}
}

func TestConfirmMatchText(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args string
		want string
	}{
		{"bash keeps the full command", "bash", `{"command":"go test\n./..."}`, "bash go test\n./..."},
		{"filepath", "write", `{"filepath":"a/b.go","content":"x"}`, "write a/b.go"},
		{"url", "fetch", `{"url":"https://example.com/x"}`, "fetch https://example.com/x"},
		{"query", "web_search", `{"query":"golang"}`, "web_search golang"},
		{"unknown uses compact json", "generate_image", `{"prompt": "a cat",  "n": 2}`, `generate_image {"prompt": "a cat", "n": 2}`},
		{"no args", "some_tool", `{}`, "some_tool {}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := confirmMatchText(tt.tool, tt.args); got != tt.want {
				t.Errorf("confirmMatchText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfirmDetail(t *testing.T) {
	cmd := "go test ./...\ngo vet ./..."
	if got := confirmDetail("bash", `{"command":"go test ./...\ngo vet ./..."}`); got != cmd {
		t.Errorf("confirmDetail(bash) = %q, want the full command", got)
	}
	if got := confirmDetail("write", `{"filepath":"a.go","content":"x"}`); got != "" {
		t.Errorf("confirmDetail(write) = %q, want empty", got)
	}
}
