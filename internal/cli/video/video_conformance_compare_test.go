package video

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
)

// This file compares a plan's preview body with the body a real client sent:
// render the preview the way --dry-run does (HTML escaping off), resolve its
// upload tokens to the URLs the upload step returned, then compare semantically.

// marshalPreviewBody renders a plan body the way --dry-run/--verbose does: JSON
// with HTML escaping off, so <UPLOAD_URL_i> tokens survive verbatim.
func marshalPreviewBody(preview any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(preview); err != nil {
		return nil, fmt.Errorf("marshal preview body: %w", err)
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// equalJSON reports whether two JSON documents carry the same value. Key order,
// whitespace and HTML escaping are transport, not contract, so they are ignored.
func equalJSON(a, b []byte) bool {
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

// substituteUploadTokens replaces each <UPLOAD_URL_i> token with the URL the
// upload step resolved for slot i, mirroring what the real send carries.
func substituteUploadTokens(preview []byte, resolved []string) ([]byte, error) {
	out := preview
	for i, u := range resolved {
		token := []byte(reqbuild.Placeholder(i))
		if !bytes.Contains(out, token) {
			return nil, fmt.Errorf("preview body lacks upload token %s: %s", token, out)
		}
		out = bytes.ReplaceAll(out, token, []byte(u))
	}
	return out, nil
}

// previewMatchesSent returns the token-resolved preview, whether it equals the
// body actually sent, and any substitution error.
func previewMatchesSent(preview []byte, resolved []string, sent string) ([]byte, bool, error) {
	want, err := substituteUploadTokens(preview, resolved)
	if err != nil {
		return nil, false, err
	}
	return want, equalJSON(want, []byte(sent)), nil
}

// requirePreviewMatchesSent fails unless the preview body the command renders
// equals the body the client actually put on the wire.
func requirePreviewMatchesSent(t *testing.T, preview any, resolved []string, sent string) {
	t.Helper()
	raw, err := marshalPreviewBody(preview)
	if err != nil {
		t.Fatalf("marshal preview body: %v", err)
	}
	want, ok, err := previewMatchesSent(raw, resolved, sent)
	if err != nil {
		t.Fatalf("preview body cannot be matched to the send: %v", err)
	}
	if !ok {
		t.Errorf("preview body drifted from the body actually sent\n  preview (tokens resolved): %s\n  sent: %s", want, sent)
	}
}

// assertSentBody fails when a sent body still carries a preview token, misses
// the uploaded URL, or leaks any local path that should have been uploaded.
func assertSentBody(t *testing.T, sent string, localPaths ...string) {
	t.Helper()
	if strings.Contains(sent, "<UPLOAD_URL") {
		t.Errorf("sent body leaked a preview token: %s", sent)
	}
	if len(localPaths) == 0 {
		return
	}
	if !strings.Contains(sent, conformanceUploadedURL) {
		t.Errorf("sent body lacks uploaded URL %q: %s", conformanceUploadedURL, sent)
	}
	for _, path := range localPaths {
		if strings.Contains(sent, path) {
			t.Errorf("sent body leaked local path %q: %s", path, sent)
		}
	}
}

// TestVideoConformanceComparisonIsSensitive is the self-check for the
// comparison itself: a mutated expected body must not compare equal, otherwise
// the provider tests would pass vacuously.
func TestVideoConformanceComparisonIsSensitive(t *testing.T) {
	preview := []byte(`{"model":"m","image_urls":["<UPLOAD_URL_0>"]}`)
	sent := `{"model":"m","image_urls":["` + conformanceUploadedURL + `"]}`

	_, ok, err := previewMatchesSent(preview, []string{conformanceUploadedURL}, sent)
	if err != nil {
		t.Fatalf("baseline previewMatchesSent() error = %v", err)
	}
	if !ok {
		t.Fatal("baseline: a resolved preview should match the sent body")
	}

	mutations := []struct {
		name     string
		preview  []byte
		resolved []string
	}{
		{"mutated model", []byte(`{"model":"MUTATED","image_urls":["<UPLOAD_URL_0>"]}`), []string{conformanceUploadedURL}},
		{"mutated upload URL", preview, []string{"https://cdn.example/other.png"}},
		{"dropped field", []byte(`{"model":"m"}`), nil},
	}
	for _, m := range mutations {
		_, ok, err := previewMatchesSent(m.preview, m.resolved, sent)
		if err != nil {
			t.Fatalf("%s: previewMatchesSent() error = %v", m.name, err)
		}
		if ok {
			t.Errorf("%s: comparison must reject a mutated expectation", m.name)
		}
	}
}
