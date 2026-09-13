package cmd

import (
	"sync"

	"github.com/martianzhang/aigc-cli/internal/client"
)

var (
	mjClientOnce sync.Once
	mjClientInst client.APIClient
)

// newMJClient returns a singleton Midjourney API client, created once with
// the current resolved provider and MJ-specific timeout. Reuses the same client
// across all 27+ MJ subcommands instead of creating a new one each time.
func newMJClient() client.APIClient {
	mjClientOnce.Do(func() {
		p := shared.ResolveProvider(ProviderNameMidjourney)
		mjClientInst = client.NewFromProvider(p)
		applyTimeout(mjClientInst, "midjourney", client.MJTimeout)
	})
	return mjClientInst
}

// ============================================================================
// MJ shared flag variables
// ============================================================================
var (
	mjPrompt    string
	mjImageURLs []string
	mjTaskID    string
	mjIndex     int
	mjCustomID  string
	mjSpeed     string
	mjDryRun    bool
	mjJSONInput string
)

// MJ imagine (structured) flag variables
var (
	mjSize      string
	mjQuality   string
	mjStyle     string
	mjVersion   string
	mjSeed      int
	mjNegPrompt string
	mjStylize   int
	mjChaos     int
	mjWeird     int
	mjTile      bool
	mjNiji      bool
	mjIw        float64
	mjCw        int
	mjSw        int
	mjCref      string
	mjSref      string
	mjDref      string
	mjDw        float64
	mjRepeat    int
	mjRaw       bool
	mjDraft     bool
	mjHd        bool
	mjStop      int
	mjExtra     string
)
