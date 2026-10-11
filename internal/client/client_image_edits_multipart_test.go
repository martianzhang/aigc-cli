package client

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// writeEditsPNG writes a tiny decodable PNG and returns its path plus bytes.
func writeEditsPNG(t *testing.T) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "in.png")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path, buf.Bytes()
}

func TestBuildImageEditsMultipart(t *testing.T) {
	local, raw := writeEditsPNG(t)
	n := 2
	req := &types.GenerateRequest{
		Model:     "openai/gpt-image-2.5-sunburst",
		Prompt:    "make it crayon",
		Size:      "1024x768",
		Quality:   "high",
		N:         &n,
		ImageURLs: []string{local},
	}

	contentType, body, err := buildImageEditsMultipart(req)
	if err != nil {
		t.Fatalf("buildImageEditsMultipart() error = %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parse content type %q: %v", contentType, err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want multipart/form-data", mediaType)
	}

	fields := map[string]string{}
	var fileData []byte
	var fileField, fileType string
	mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		data, _ := io.ReadAll(part)
		if part.FileName() != "" {
			fileData, fileField, fileType = data, part.FormName(), part.Header.Get("Content-Type")
			continue
		}
		fields[part.FormName()] = string(data)
	}

	want := map[string]string{
		"model":         "openai/gpt-image-2.5-sunburst",
		"prompt":        "make it crayon",
		"size":          "1024x768",
		"quality":       "high",
		"n":             "2",
		"output_format": "",
	}
	for k, v := range want {
		if k == "output_format" {
			if _, ok := fields[k]; ok {
				t.Errorf("unset optional field %q should be absent", k)
			}
			continue
		}
		if fields[k] != v {
			t.Errorf("field %q = %q, want %q", k, fields[k], v)
		}
	}
	if fileField != "image" {
		t.Errorf("file field = %q, want image", fileField)
	}
	if fileType != "image/png" {
		t.Errorf("file content type = %q, want image/png", fileType)
	}
	if !bytes.Equal(fileData, raw) {
		t.Errorf("file bytes = %d, want %d", len(fileData), len(raw))
	}
}

func TestBuildImageEditsMultipartDataURI(t *testing.T) {
	_, raw := writeEditsPNG(t)
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
	req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{dataURI}}

	_, body, err := buildImageEditsMultipart(req)
	if err != nil {
		t.Fatalf("buildImageEditsMultipart() error = %v", err)
	}
	if !bytes.Contains(body, raw) {
		t.Error("decoded data URI bytes should be attached as a file part")
	}
}

func TestDoBytesAppliesExtraHeaders(t *testing.T) {
	var gotPin string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPin = r.Header.Get("X-OfoxAI-Provider-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[]}`))
	}))
	defer srv.Close()

	c := NewWithProvider("test-key", srv.URL, "", types.ProviderOpenAI)
	var result types.OpenAIImageResponse
	err := c.doBytes(http.MethodPost, "/x", "application/json", []byte(`{}`), &result,
		map[string]string{"X-OfoxAI-Provider-Type": "openai"})
	if err != nil {
		t.Fatalf("doBytes() error = %v", err)
	}
	if gotPin != "openai" {
		t.Errorf("X-OfoxAI-Provider-Type = %q, want openai", gotPin)
	}
}

func TestImageEditsFormFieldsFromJSON(t *testing.T) {
	raw := []byte(`{"prompt":"p","model":"m","n":2,"flag":true,"empty":"","image_urls":["a.png"],"loras":["x","y"],"nested":{"a":1}}`)
	fields := ImageEditsFormFields(&types.GenerateRequest{RawJSON: raw})

	var names []string
	values := map[string]string{}
	var loras []string
	for _, f := range fields {
		names = append(names, f.Name)
		if f.Name == "loras" {
			loras = append(loras, f.Value)
			continue
		}
		values[f.Name] = f.Value
	}

	wantNames := "flag,loras,loras,model,n,prompt"
	if got := strings.Join(names, ","); got != wantNames {
		t.Errorf("field order = %v, want %s", names, wantNames)
	}
	if values["flag"] != "true" || values["model"] != "m" || values["n"] != "2" || values["prompt"] != "p" {
		t.Errorf("scalar fields = %v", values)
	}
	if got := strings.Join(loras, ","); got != "x,y" {
		t.Errorf("array field = %v, want x,y", loras)
	}
	for _, skip := range []string{"image_urls", "nested", "empty"} {
		if _, ok := values[skip]; ok {
			t.Errorf("%q must not become a form field", skip)
		}
	}
}

func TestImageGenerateEditsMultipartSends(t *testing.T) {
	local, _ := writeEditsPNG(t)

	var gotPath, gotCT, gotModel string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		mr, err := r.MultipartReader()
		if err != nil {
			t.Errorf("multipart reader: %v", err)
			return
		}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("read part: %v", err)
				return
			}
			if part.FormName() == "model" {
				b, _ := io.ReadAll(part)
				gotModel = string(b)
				continue
			}
			_, _ = io.Copy(io.Discard, part)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1,"data":[]}`))
	}))
	defer srv.Close()

	c := NewWithProvider("test-key", srv.URL, "", types.ProviderOpenAI)
	req := &types.GenerateRequest{Model: "m", Prompt: "p", ImageURLs: []string{local}}
	if _, err := c.ImageGenerateEditsMultipart(req); err != nil {
		t.Fatalf("ImageGenerateEditsMultipart() error = %v", err)
	}
	if gotPath != "/v1"+ImageEditsPath {
		t.Errorf("path = %q, want /v1%s", gotPath, ImageEditsPath)
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data") {
		t.Errorf("content type = %q, want multipart/form-data", gotCT)
	}
	if gotModel != "m" {
		t.Errorf("model field = %q, want m", gotModel)
	}
}
