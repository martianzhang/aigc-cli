package search

import (
	"github.com/martianzhang/aigc-cli/internal/reqbuild"
	"github.com/martianzhang/aigc-cli/internal/service"
)

// Previewer is an optional extension for providers that can render the
// equivalent HTTP request without sending it. `aigc-cli search --dry-run` uses
// it to print a copy-pasteable curl command.
type Previewer interface {
	Provider
	Preview(query string, limit int, opts Options) (*reqbuild.Plan, error)
}

// maskedKey masks an API key for curl previews.
func maskedKey(key string) string { return service.MaskKey(key) }
