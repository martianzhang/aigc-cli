package reqbuild

import (
	"encoding/json"
	"strings"
	"testing"
)

func FuzzRenderCurls(f *testing.F) {
	f.Add("POST", "https://api.example.com/v1/images/generations", `{"a":1}`, "sk-test")
	f.Add("GET", "http://127.0.0.1:11434/v1/models", "", "")
	f.Add("", "", "not-json", "")

	f.Fuzz(func(t *testing.T, method, url, body, key string) {
		if method == "" {
			method = "GET"
		}
		p := &Plan{
			Method:  method,
			URL:     url,
			Body:    json.RawMessage(body),
			Stream:  method != "GET",
			Headers: []Header{{Name: "X-Test", Value: key}},
		}
		out, err := p.RenderCurlsErr(key)
		if err != nil {
			return
		}
		if !strings.HasPrefix(out, "curl") {
			t.Fatalf("rendered preview does not start with curl: %q", out)
		}
	})
}
