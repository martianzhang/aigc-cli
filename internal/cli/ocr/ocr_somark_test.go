package ocr

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/provider"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// somarkTestServer answers /parse/sync with the given markdown and captures
// the multipart form values of the request.
func somarkTestServer(t *testing.T, markdown string, capture *map[string][]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/parse/sync" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if capture != nil {
			if err := r.ParseMultipartForm(4 << 20); err != nil {
				t.Errorf("parse multipart: %v", err)
			} else {
				*capture = r.MultipartForm.Value
			}
		}
		io.WriteString(w, `{"code":0,"data":{"metadata":{"page_num":1,"file_type":".pdf"},
			"result":{"outputs":{"markdown":`+quoteJSON(t, markdown)+`}}}}`)
	}))
}

func quoteJSON(t *testing.T, s string) string {
	t.Helper()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// withShared swaps the process-wide option state for one test.
func withShared(t *testing.T, sc options.SharedConfig) {
	t.Helper()
	old := *options.Shared
	*options.Shared = sc
	t.Cleanup(func() { *options.Shared = old })
}

func writeTestDocument(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("%PDF-1.4 test"), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScanWithSomark_WritesMarkdown(t *testing.T) {
	const markdown = "# Title\n\nbody text"
	srv := somarkTestServer(t, markdown, nil)
	defer srv.Close()

	withShared(t, options.SharedConfig{})
	input := writeTestDocument(t, "doc.pdf")
	p := &provider.EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1"}
	if err := scanWithSomark(nil, p, input); err != nil {
		t.Fatalf("scanWithSomark: %v", err)
	}

	md, err := os.ReadFile(filepath.Join(filepath.Dir(input), "doc.md"))
	if err != nil {
		t.Fatalf("read markdown output: %v", err)
	}
	if !strings.Contains(string(md), markdown) {
		t.Errorf("markdown output = %q", md)
	}
}

func TestScanWithSomark_SendsJSONBody(t *testing.T) {
	var fields map[string][]string
	srv := somarkTestServer(t, "ok", &fields)
	defer srv.Close()

	withShared(t, options.SharedConfig{
		JSONInput:  `{"output_formats":["markdown"],"element_formats":{"table":"html"}}`,
		InjectJSON: `{"feature_config":{"enable_title_level_recognition":true}}`,
	})
	input := writeTestDocument(t, "doc.pdf")
	p := &provider.EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1"}
	if err := scanWithSomark(nil, p, input); err != nil {
		t.Fatalf("scanWithSomark: %v", err)
	}

	if got := fields["output_formats"]; len(got) != 1 || got[0] != "markdown" {
		t.Errorf("output_formats = %v, --json must replace the defaults", got)
	}
	if got := fields["element_formats"][0]; !strings.Contains(got, `"table":"html"`) {
		t.Errorf("element_formats = %v", got)
	}
	if got := fields["feature_config"][0]; !strings.Contains(got, "enable_title_level_recognition") {
		t.Errorf("feature_config = %v, --inject-json must be merged in", got)
	}
}

func TestScanWithSomark_LocalizesImages(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/parse/sync":
			markdown := "# Figure\n\n![fig](" + srv.URL + "/fig.png)\n"
			io.WriteString(w, `{"code":0,"data":{"metadata":{"page_num":1},"result":{"outputs":{"markdown":`+
				quoteJSON(t, markdown)+`}}}}`)
		case "/fig.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(somarkTestPNG)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	withShared(t, options.SharedConfig{})
	input := writeTestDocument(t, "doc.pdf")
	p := &provider.EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1"}
	if err := scanWithSomark(nil, p, input); err != nil {
		t.Fatalf("scanWithSomark: %v", err)
	}

	dir := filepath.Dir(input)
	md, err := os.ReadFile(filepath.Join(dir, "doc.md"))
	if err != nil {
		t.Fatalf("read markdown output: %v", err)
	}
	if !strings.Contains(string(md), "doc.assets/image_001.png") {
		t.Errorf("markdown = %s, want a rewritten local image link", md)
	}
	if _, err := os.Stat(filepath.Join(dir, "doc.assets", "image_001.png")); err != nil {
		t.Errorf("expected the image next to the markdown: %v", err)
	}
}

// somarkTestPNG is a minimal 1x1 PNG used by the image localization test.
var somarkTestPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
	0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41,
	0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
	0x00, 0x00, 0x03, 0x00, 0x01, 0x26, 0xE0, 0xFE,
	0x69, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E,
	0x44, 0xAE, 0x42, 0x60, 0x82,
}

func TestScanWithSomark_DownloadsZip(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/parse/sync":
			io.WriteString(w, `{"code":0,"data":{"metadata":{"page_num":1},
				"result":{"outputs":{"zip":"`+srv.URL+`/doc.zip"}}}}`)
		case "/doc.zip":
			w.Header().Set("Content-Type", "application/zip")
			io.WriteString(w, "PK-fake-zip")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	withShared(t, options.SharedConfig{})
	input := writeTestDocument(t, "doc.pdf")
	p := &provider.EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1"}
	if err := scanWithSomark(nil, p, input); err != nil {
		t.Fatalf("scanWithSomark: %v", err)
	}

	dir := filepath.Dir(input)
	data, err := os.ReadFile(filepath.Join(dir, "doc.zip"))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	if string(data) != "PK-fake-zip" {
		t.Errorf("zip content = %q", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "doc.md")); err == nil {
		t.Error("a zip export must not also write a bare .md file")
	}
}

func TestScanWithSomark_StdoutAndWarnings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"code":0,"warnings":["enable_title_level_recognition is ignored"],
			"data":{"metadata":{"page_num":1},"result":{"outputs":{"markdown":"# T\n\nbody"}}}}`)
	}))
	defer srv.Close()

	withShared(t, options.SharedConfig{OutputDir: "-"})
	oldPreview := ocrScanPreview
	ocrScanPreview = true
	defer func() { ocrScanPreview = oldPreview }()

	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdoutWriter, stderrWriter
	input := writeTestDocument(t, "doc.png")
	err = scanWithSomark(nil, &provider.EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1"}, input)
	os.Stdout, os.Stderr = origOut, origErr
	stdoutWriter.Close()
	stderrWriter.Close()

	out, _ := io.ReadAll(stdout)
	stdout.Close()
	errOut, _ := io.ReadAll(stderrReader)
	stderrReader.Close()

	if err != nil {
		t.Fatalf("scanWithSomark: %v", err)
	}
	if !strings.Contains(string(out), "body") {
		t.Errorf("stdout = %q, want the parsed markdown", out)
	}
	if !strings.Contains(string(errOut), "is ignored") {
		t.Errorf("stderr = %q, want the API warning", errOut)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(input), "doc.md")); statErr == nil {
		t.Error("-o - must not write a file")
	}
}

func TestScanWithSomark_RejectsStdinJSON(t *testing.T) {
	withShared(t, options.SharedConfig{JSONInput: "-"})
	p := &provider.EffectiveProvider{Type: types.ProviderSomark, BaseURL: "http://127.0.0.1:1/v1"}
	err := scanWithSomark(nil, p, writeTestDocument(t, "doc.pdf"))
	if err == nil || !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("err = %v, want a stdin rejection", err)
	}
}

func TestScanWithSomark_IgnoredFlagWarning(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("prompt", "", "")
	cmd.Flags().String("model", "", "")
	cmd.Flags().String("pages", "", "")
	if err := cmd.Flags().Set("prompt", "Free OCR."); err != nil {
		t.Fatal(err)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = writer
	warnIgnoredFlags(cmd)
	os.Stderr = origStderr
	writer.Close()
	out, _ := io.ReadAll(reader)
	reader.Close()

	text := string(out)
	if !strings.Contains(text, "--prompt") {
		t.Errorf("expected a warning for --prompt, got %q", text)
	}
	if strings.Contains(text, "--model") {
		t.Errorf("unset flags must not warn, got %q", text)
	}

	// A nil command (unit tests, programmatic use) must not panic.
	warnIgnoredFlags(nil)
}

func TestScanWithSomark_ErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, `{"code":1137,"message":"invalid document"}`)
	}))
	defer srv.Close()

	withShared(t, options.SharedConfig{})
	input := writeTestDocument(t, "doc.pdf")
	p := &provider.EffectiveProvider{Type: types.ProviderSomark, BaseURL: srv.URL + "/v1"}
	err := scanWithSomark(nil, p, input)
	if err == nil || !strings.Contains(err.Error(), "1137") {
		t.Fatalf("err = %v, want the API error code", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(input), "doc.md")); statErr == nil {
		t.Error("no output file should be written on failure")
	}
}
