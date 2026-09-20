package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// newResourceTestConfig returns a Config with empty defaults, safe for NewServer.
func newResourceTestConfig(output string) *Config {
	return &Config{
		Output:   output,
		Defaults: &types.ConfigDefaults{},
	}
}

// readResource invokes a resource handler and returns its contents.
func readResource(t *testing.T, handler server.ResourceHandlerFunc, uri string) []mcp.ResourceContents {
	t.Helper()
	contents, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err != nil {
		t.Fatalf("read %s: %v", uri, err)
	}
	return contents
}

// readResourceErr invokes a resource handler and requires it to fail.
func readResourceErr(t *testing.T, handler server.ResourceHandlerFunc, uri string) error {
	t.Helper()
	_, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err == nil {
		t.Fatalf("read %s: expected an error, got nil", uri)
	}
	return err
}

// textOf asserts a single text contents block and returns its text.
func textOf(t *testing.T, contents []mcp.ResourceContents) string {
	t.Helper()
	if len(contents) != 1 {
		t.Fatalf("contents count = %d, want 1", len(contents))
	}
	text, ok := contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("contents[0] type = %T, want TextResourceContents", contents[0])
	}
	return text.Text
}

// writeFile writes content to path, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestRegisterResources_registersResourcesAndTemplate(t *testing.T) {
	s := NewServer(newResourceTestConfig(t.TempDir()))

	want := map[string]bool{
		providersResourceURI: false,
		configResourceURI:    false,
		outputResourceURI:    false,
	}
	for uri := range s.ListResources() {
		if _, ok := want[uri]; ok {
			want[uri] = true
		}
	}
	for uri, found := range want {
		if !found {
			t.Errorf("resource %s not registered", uri)
		}
	}

	resp, ok := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"resources/templates/list"}`)).(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("resources/templates/list: unexpected response type %T", resp)
	}
	list, ok := resp.Result.(mcp.ListResourceTemplatesResult)
	if !ok {
		t.Fatalf("resources/templates/list: unexpected result type %T", resp.Result)
	}
	for _, tmpl := range list.ResourceTemplates {
		if tmpl.URITemplate != nil && tmpl.URITemplate.Raw() == outputTemplateURI {
			return
		}
	}
	t.Errorf("resource template %s not registered", outputTemplateURI)
}

func TestRegisterResources_templateReadIsRouted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "note.txt"), "hello resource")
	s := NewServer(newResourceTestConfig(dir))

	resp, ok := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"aigc://output/note.txt"}}`)).(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("resources/read: unexpected response type %T", resp)
	}
	result, ok := resp.Result.(mcp.ReadResourceResult)
	if !ok {
		t.Fatalf("resources/read: unexpected result type %T", resp.Result)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("contents count = %d, want 1", len(result.Contents))
	}
	text, ok := result.Contents[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("contents[0] type = %T, want TextResourceContents", result.Contents[0])
	}
	if text.Text != "hello resource" {
		t.Errorf("Text = %q, want %q", text.Text, "hello resource")
	}

	// A traversal attempt must never yield contents, whether the URI fails to
	// match the template or the handler rejects it.
	bad := s.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"aigc://output/../secret"}}`))
	if resp, ok := bad.(mcp.JSONRPCResponse); ok {
		if result, ok := resp.Result.(mcp.ReadResourceResult); ok && len(result.Contents) > 0 {
			t.Fatalf("traversal read unexpectedly succeeded: %+v", result.Contents)
		}
	}
}

func TestProvidersResource_listsNamesWithoutSecrets(t *testing.T) {
	cfg := &Config{Providers: map[string]*types.NamedProvider{
		"zeta":  {APIKey: "sk-zeta-secret", BaseURL: "https://user:pass@zeta.example.com:8443/v1?token=abc"},
		"alpha": {Type: types.ProviderOpenAI, BaseURL: "https://alpha.example.com/v1"},
	}}

	text := textOf(t, readResource(t, providersResourceHandler(cfg), providersResourceURI))

	for _, want := range []string{`"alpha"`, `"zeta"`, "alpha.example.com", "zeta.example.com"} {
		if !strings.Contains(text, want) {
			t.Errorf("providers resource missing %q:\n%s", want, text)
		}
	}
	if strings.Index(text, `"alpha"`) > strings.Index(text, `"zeta"`) {
		t.Errorf("provider names should be sorted:\n%s", text)
	}
	for _, secret := range []string{"sk-zeta-secret", "user:pass", "token=abc", "8443"} {
		if strings.Contains(text, secret) {
			t.Errorf("providers resource leaked %q:\n%s", secret, text)
		}
	}
}

func TestProvidersResource_empty(t *testing.T) {
	text := textOf(t, readResource(t, providersResourceHandler(&Config{}), providersResourceURI))
	if !strings.Contains(text, `"providers": []`) {
		t.Errorf("empty config should list no providers:\n%s", text)
	}
}

func TestOutputResource_listsTopLevelFilesOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "b.txt"), "bb")
	writeFile(t, filepath.Join(dir, "a.png"), "aa")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "sub", "inner.txt"), "inner")

	text := textOf(t, readResource(t, outputResourceHandler(&Config{Output: dir}), outputResourceURI))

	for _, want := range []string{"a.png", "b.txt", "Output directory: " + dir} {
		if !strings.Contains(text, want) {
			t.Errorf("listing missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "inner.txt") || strings.Contains(text, "- sub") {
		t.Errorf("listing should be non-recursive and skip directories:\n%s", text)
	}
	if strings.Index(text, "a.png") > strings.Index(text, "b.txt") {
		t.Errorf("listing should be sorted by name:\n%s", text)
	}
}

func TestOutputResource_noOutputDirectory(t *testing.T) {
	text := textOf(t, readResource(t, outputResourceHandler(&Config{}), outputResourceURI))
	if !strings.Contains(text, "no output directory configured") {
		t.Errorf("text = %q, want the no-output-directory notice", text)
	}
}

func TestOutputResource_capsListing(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < maxResourceEntries+5; i++ {
		writeFile(t, filepath.Join(dir, fmt.Sprintf("f%03d.txt", i)), "x")
	}

	text := textOf(t, readResource(t, outputResourceHandler(&Config{Output: dir}), outputResourceURI))

	if got := strings.Count(text, "\n- "); got != maxResourceEntries {
		t.Errorf("listed entries = %d, want %d", got, maxResourceEntries)
	}
	if !strings.Contains(text, "... and 5 more (listing capped at 200 entries)") {
		t.Errorf("missing truncation note:\n%s", text)
	}
	if strings.Contains(text, "f204.txt") {
		t.Errorf("listing should not include entries past the cap:\n%s", text)
	}
}
