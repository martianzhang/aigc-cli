package redact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	redactlib "github.com/martianzhang/aigc-cli/internal/redact"
)

func TestTokenclsModelRegistry(t *testing.T) {
	info, ok := redactlib.FindTokenclsModel("zh-pii")
	if !ok {
		t.Fatal("zh-pii model missing")
	}
	if info.License != "Apache-2.0" || !strings.Contains(info.Source, "privyscope") {
		t.Fatalf("model metadata = %+v", info)
	}
	names := map[string]bool{}
	for _, file := range info.Files {
		names[file.OutName] = true
	}
	for _, want := range []string{"model.onnx", "tokenizer.json", "config.json", "LICENSE.txt"} {
		if !names[want] {
			t.Fatalf("model files = %v, missing %s", names, want)
		}
	}
}

func TestDownloadTokenclsFiles(t *testing.T) {
	files := map[string]string{
		"/model.onnx":     "fake-onnx",
		"/tokenizer.json": `{"tokenizer":true}`,
		"/config.json":    `{"id2label":{"0":"O"}}`,
	}
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	info := redactlib.TokenclsModelInfo{ID: "test", Files: []redactlib.TokenclsModelFile{}}
	for name, body := range files {
		out := strings.TrimPrefix(name, "/")
		sum := sha256.Sum256([]byte(body))
		info.Files = append(info.Files, redactlib.TokenclsModelFile{
			URL: server.URL + name, OutName: out, SHA256: hex.EncodeToString(sum[:]),
		})
	}

	dir := t.TempDir()
	var out bytes.Buffer
	if err := downloadTokenclsFiles(&out, dir, info, false); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		got, err := os.ReadFile(filepath.Join(dir, strings.TrimPrefix(name, "/")))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != body {
			t.Fatalf("%s = %q, want %q", name, got, body)
		}
	}
	before := hits.Load()

	// A second run must reuse matching files without extra requests.
	out.Reset()
	if err := downloadTokenclsFiles(&out, dir, info, false); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != before {
		t.Fatalf("second install downloaded %d extra file(s)", hits.Load()-before)
	}

	// A corrupted file is re-downloaded (checksum mismatch is repaired).
	corrupt := filepath.Join(dir, "model.onnx")
	if err := os.WriteFile(corrupt, []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := downloadTokenclsFiles(io.Discard, dir, info, false); err != nil {
		t.Fatalf("corrupt install error = %v", err)
	}
	got, _ := os.ReadFile(corrupt)
	if string(got) != "fake-onnx" {
		t.Fatalf("repair install left %q", got)
	}
}

func TestWriteTokenclsModelsTable(t *testing.T) {
	var out bytes.Buffer
	if err := writeTokenclsModelsTable(&out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"zh-pii", "Apache-2.0", "privyscope", "multilingual-ner", "AFL-3.0"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("table missing %q:\n%s", want, out.String())
		}
	}
}

func TestInstallTokenclsModelInfo(t *testing.T) {
	body := "fake-model"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	sum := sha256.Sum256([]byte(body))
	info := redactlib.TokenclsModelInfo{
		ID: "test", Name: "Test Model", License: "Apache-2.0",
		Source: "https://example.com/upstream",
		Files: []redactlib.TokenclsModelFile{{
			URL: server.URL + "/model.onnx", OutName: "model.onnx",
			SHA256: hex.EncodeToString(sum[:]),
		}},
	}
	root := t.TempDir()
	var out, progress bytes.Buffer
	if err := installTokenclsModelInfo(&out, &progress, info, root, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "redact/test/model.onnx")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Source:   https://example.com/upstream") ||
		!strings.Contains(out.String(), "model: test") {
		t.Fatalf("output = %s", out.String())
	}
	if !strings.Contains(progress.String(), "Installing Test Model") {
		t.Fatalf("progress = %s", progress.String())
	}
}

func TestInstallTokenclsModelUnknownID(t *testing.T) {
	err := installTokenclsModel(nil, "nope", "", false)
	if err == nil || !strings.Contains(err.Error(), "available: zh-pii, multilingual-ner") {
		t.Fatalf("error = %v", err)
	}
}

func TestDownloadTokenclsFilesFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()
	info := redactlib.TokenclsModelInfo{ID: "test", Files: []redactlib.TokenclsModelFile{{
		URL: server.URL + "/missing", OutName: "model.onnx",
	}}}
	if err := downloadTokenclsFiles(io.Discard, t.TempDir(), info, false); err == nil ||
		!strings.Contains(err.Error(), "install model.onnx") {
		t.Fatalf("download error = %v", err)
	}

	body := "wrong"
	sum := sha256.Sum256([]byte("expected"))
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server2.Close()
	info = redactlib.TokenclsModelInfo{ID: "test", Files: []redactlib.TokenclsModelFile{{
		URL: server2.URL + "/model.onnx", OutName: "model.onnx",
		SHA256: hex.EncodeToString(sum[:]),
	}}}
	if err := downloadTokenclsFiles(io.Discard, t.TempDir(), info, false); err == nil ||
		!strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("checksum error = %v", err)
	}
}

func TestWriteSetup(t *testing.T) {
	var out bytes.Buffer
	writeSetup(&out)
	for _, want := range []string{"redact init --model zh-pii", "Apache-2.0", "privyscope-zh-hans"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("setup missing %q:\n%s", want, out.String())
		}
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := expandHome("~/models"); got != filepath.Join(home, "models") {
		t.Fatalf("expandHome = %q", got)
	}
	if got := expandHome("/tmp/models"); got != "/tmp/models" {
		t.Fatalf("absolute path = %q", got)
	}
}

func TestMultilingualModelPack(t *testing.T) {
	info, ok := redactlib.FindTokenclsModel("multilingual-ner")
	if !ok {
		t.Fatal("multilingual-ner pack missing")
	}
	if info.License != "AFL-3.0" || !strings.Contains(info.Source, "aigc-cli-models") {
		t.Fatalf("pack metadata = %+v", info)
	}
	var modelFile bool
	for _, f := range info.Files {
		if f.OutName == "model.onnx" {
			modelFile = true
			if f.SHA256 == "" || f.SizeMB < 100 {
				t.Fatalf("model file metadata = %+v", f)
			}
		}
	}
	if !modelFile {
		t.Fatal("model.onnx missing from pack")
	}
}
