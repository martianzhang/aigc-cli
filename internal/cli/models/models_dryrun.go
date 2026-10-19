package models

import (
	"fmt"
	"net/http"

	"github.com/martianzhang/aigc-cli/internal/reqbuild"
)

// printModelsCurl renders the equivalent curl for a models-command GET
// request. The API key is masked, and note (when non-empty) is appended as a
// comment.
func printModelsCurl(url, apiKey, note string) {
	plan := &reqbuild.Plan{Method: http.MethodGet, URL: url, Note: note}
	fmt.Println(plan.RenderCurls(apiKey))
}
