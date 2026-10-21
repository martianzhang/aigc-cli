package search

import "github.com/martianzhang/aigc-cli/internal/reqbuild"

// FetchPreviewer is an optional extension for fetch-capable providers that can
// render the equivalent HTTP request without sending it. `aigc-cli fetch
// --dry-run` uses it to print a copy-pasteable curl command.
type FetchPreviewer interface {
	Fetcher
	PreviewFetch(url string) (*reqbuild.Plan, error)
}
