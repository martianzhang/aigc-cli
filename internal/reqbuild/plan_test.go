package reqbuild

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

const (
	testUploadURL = "https://api.apimart.ai/v1/uploads/images"
	testGenURL    = "https://api.apimart.ai/v1/images/generations"
	testAPIKey    = "sk-test"
	testMasked    = "...test"
)

func TestPlaceholder(t *testing.T) {
	if got, want := Placeholder(0), "<UPLOAD_URL_0>"; got != want {
		t.Errorf("Placeholder(0) = %q, want %q", got, want)
	}
	if got, want := Placeholder(12), "<UPLOAD_URL_12>"; got != want {
		t.Errorf("Placeholder(12) = %q, want %q", got, want)
	}
}

// twoUploadPlan is the canonical POST plan with one upload per placeholder.
func twoUploadPlan() *Plan {
	return &Plan{
		Method:    http.MethodPost,
		URL:       testGenURL,
		Body:      map[string]any{"model": "m", "image_urls": []string{Placeholder(0), Placeholder(1)}},
		Uploads:   []Upload{{Path: "a.png", Label: "image_urls[0]"}, {Path: "b.png", Label: "image_urls[1]"}},
		UploadURL: testUploadURL,
	}
}

func TestRenderCurlsPostWithUploads(t *testing.T) {
	p := twoUploadPlan()
	got := p.RenderCurls(testAPIKey)

	want := strings.Join([]string{
		"curl -X POST " + testUploadURL + " \\",
		`  -H "Authorization: Bearer ` + testMasked + `" \`,
		`  -F "file=@a.png"`,
		"curl -X POST " + testUploadURL + " \\",
		`  -H "Authorization: Bearer ` + testMasked + `" \`,
		`  -F "file=@b.png"`,
		"curl -X POST " + testGenURL + " \\",
		`  -H "Authorization: Bearer ` + testMasked + `" \`,
		`  -H "Content-Type: application/json" \`,
		`  -d '{"image_urls":["<UPLOAD_URL_0>","<UPLOAD_URL_1>"],"model":"m"}'`,
	}, "\n")
	if got != want {
		t.Errorf("RenderCurls() =\n%s\nwant:\n%s", got, want)
	}

	if n := strings.Count(got, `-F "file=@`); n != 2 {
		t.Errorf("multipart -F count = %d, want 2", n)
	}
	if n := strings.Count(got, "  -d '"); n != 1 {
		t.Errorf("-d count = %d, want 1", n)
	}
	if n := strings.Count(got, "Content-Type"); n != 1 {
		t.Errorf("Content-Type count = %d, want 1 (multipart curls must not carry it)", n)
	}
	if strings.Contains(got, testAPIKey) {
		t.Errorf("raw api key leaked into preview:\n%s", got)
	}
	if !strings.Contains(got, testMasked) {
		t.Errorf("masked api key missing from preview:\n%s", got)
	}
	for _, token := range []string{"<UPLOAD_URL_0>", "<UPLOAD_URL_1>"} {
		if !strings.Contains(got, token) {
			t.Errorf("body token %s missing from preview:\n%s", token, got)
		}
	}
}

func TestRenderCurlsNoteIsLastLine(t *testing.T) {
	p := twoUploadPlan()
	p.Note = "local image files are embedded as data: URIs"
	got := p.RenderCurls(testAPIKey)

	wantLast := "# note: local image files are embedded as data: URIs"
	lines := strings.Split(got, "\n")
	if last := lines[len(lines)-1]; last != wantLast {
		t.Errorf("last line = %q, want %q", last, wantLast)
	}
	if n := strings.Count(got, "# note:"); n != 1 {
		t.Errorf("# note count = %d, want 1", n)
	}
}

func TestRenderCurlsNoNote(t *testing.T) {
	got := twoUploadPlan().RenderCurls(testAPIKey)
	if strings.Contains(got, "# note:") {
		t.Errorf("empty Note must not emit a comment:\n%s", got)
	}
	if !strings.HasSuffix(got, `  -d '{"image_urls":["<UPLOAD_URL_0>","<UPLOAD_URL_1>"],"model":"m"}'`) {
		t.Errorf("POST preview must end with the -d line:\n%s", got)
	}
}

func TestRenderCurlsNoUploads(t *testing.T) {
	p := &Plan{Method: http.MethodPost, URL: testGenURL, Body: map[string]any{"model": "m"}}
	got := p.RenderCurls(testAPIKey)

	if strings.Contains(got, "-F \"file=@") || strings.Contains(got, testUploadURL) {
		t.Errorf("plan without uploads must not render multipart curls:\n%s", got)
	}
	if n := strings.Count(got, "  -d '"); n != 1 {
		t.Errorf("-d count = %d, want 1", n)
	}
	if !strings.Contains(got, `  -d '{"model":"m"}'`) {
		t.Errorf("body not rendered:\n%s", got)
	}
}

func TestRenderCurlsGet(t *testing.T) {
	p := &Plan{Method: http.MethodGet, URL: "https://api.apimart.ai/v1/tasks/abc"}
	got := p.RenderCurls(testAPIKey)

	want := "curl -X GET https://api.apimart.ai/v1/tasks/abc \\\n" +
		`  -H "Authorization: Bearer ` + testMasked + `"`
	if got != want {
		t.Errorf("RenderCurls() = %q, want %q", got, want)
	}
	if strings.Contains(got, "  -d '") || strings.Contains(got, "Content-Type") || strings.Contains(got, "file=@") {
		t.Errorf("GET preview must not carry a body or multipart:\n%s", got)
	}
}

func TestRenderCurlsRawBodyVerbatim(t *testing.T) {
	raw := []byte(`{"model":"m","x":1}`)

	tests := []struct {
		name string
		body any
	}{
		{"raw bytes", raw},
		{"raw message", json.RawMessage(raw)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := &Plan{Method: http.MethodPost, URL: testGenURL, Body: tc.body}
			got := p.RenderCurls(testAPIKey)
			if !strings.HasSuffix(got, "  -d '"+string(raw)+"'") {
				t.Errorf("raw body not emitted verbatim:\n%s", got)
			}
		})
	}
}

func TestRenderCurlsFieldOverride(t *testing.T) {
	p := &Plan{
		Method:    http.MethodPost,
		URL:       testGenURL,
		Body:      map[string]any{"model": "m"},
		UploadURL: testUploadURL,
		Uploads:   []Upload{{Path: "x.png", Field: "image"}},
	}
	got := p.RenderCurls(testAPIKey)
	if !strings.Contains(got, `-F "image=@x.png"`) {
		t.Errorf("custom upload field not honoured:\n%s", got)
	}
}

type fakeUploader struct {
	calls int
	got   []string
	res   []string
	err   error
}

func (f *fakeUploader) ResolveLocalImages(urls []string) ([]string, error) {
	f.calls++
	f.got = append([]string(nil), urls...)
	if f.err != nil {
		return nil, f.err
	}
	return f.res, nil
}

func TestRunUploadsOrderAndResult(t *testing.T) {
	p := twoUploadPlan()
	up := &fakeUploader{res: []string{"https://cdn/a.png", "https://cdn/b.png"}}

	got, err := p.RunUploads(up)
	if err != nil {
		t.Fatalf("RunUploads() error = %v", err)
	}
	if want := []string{"a.png", "b.png"}; !equalStrings(up.got, want) {
		t.Errorf("uploaded paths = %v, want %v", up.got, want)
	}
	if want := []string{"https://cdn/a.png", "https://cdn/b.png"}; !equalStrings(got, want) {
		t.Errorf("RunUploads() = %v, want %v", got, want)
	}
}

func TestRunUploadsNoUploadsSkipsUploader(t *testing.T) {
	p := &Plan{Method: http.MethodPost, URL: testGenURL, Body: map[string]any{"model": "m"}}
	up := &fakeUploader{}

	got, err := p.RunUploads(up)
	if err != nil {
		t.Fatalf("RunUploads() error = %v", err)
	}
	if up.calls != 0 {
		t.Errorf("uploader called %d times for plan without uploads, want 0", up.calls)
	}
	if len(got) != 0 {
		t.Errorf("RunUploads() = %v, want empty", got)
	}
}

func TestRunUploadsPropagatesError(t *testing.T) {
	sentinel := errors.New("upload boom")
	up := &fakeUploader{err: sentinel}

	if _, err := twoUploadPlan().RunUploads(up); !errors.Is(err, sentinel) {
		t.Errorf("RunUploads() error = %v, want wrapped %v", err, sentinel)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
