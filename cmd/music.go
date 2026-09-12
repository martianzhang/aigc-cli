package cmd

import (
	"sync"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/client"
)

var (
	musicClientOnce sync.Once
	musicClientInst client.APIClient
)

// newMusicClient returns a singleton music API client, created once with the
// current resolved provider and music-specific timeout.
func newMusicClient() client.APIClient {
	musicClientOnce.Do(func() {
		p := shared.ResolveProvider(ProviderNameMusic)
		musicClientInst = client.NewFromProvider(p)
		applyTimeout(musicClientInst, "music", client.MusicTimeout)
	})
	return musicClientInst
}

// ============================================================================
// Music flag variables
// ============================================================================
var (
	musicPrompt       string
	musicModel        string
	musicStyle        string
	musicTitle        string
	musicLyrics       string
	musicInstrumental bool
	musicDuration     int
	musicFormat       string
	musicJSONInput    string
	musicDryRun       bool
)

// ============================================================================
// Parent command: aigc-cli music
// ============================================================================
var musicCmd = &cobra.Command{
	Use:          "music",
	Short:        "AI music generation (APIMart: suno / flowmusic)",
	SilenceUsage: true,
	Long: `Generate music via the APIMart music API.

Music uses an async task model — you submit a job, get a task_id, then poll
for results. Both endpoints live under /v1/music/.

The --model flag selects the backend:
  suno       (default) — prompt/lyrics/style/title, optional duration/format
  flowmusic            — sound_prompt/lyrics/title/length

Subcommands:
  generate (gen)   Submit a music generation task
  query            Get music task status and result

Examples:
  aigc-cli music generate --prompt "city pop"
  aigc-cli music gen --prompt "rock" --model flowmusic
  aigc-cli music query task_xxx`,
}

// ============================================================================
// Init — register subcommands and flags
// ============================================================================
func init() {
	f := musicGenerateCmd.Flags()
	f.StringVarP(&musicPrompt, "prompt", "p", "", "Music prompt / description (or style)")
	f.StringVarP(&musicModel, "model", "m", "", "Backend model: suno (default) or flowmusic")
	f.StringVar(&musicStyle, "style", "", "Style (suno: style field; fallback for prompt)")
	f.StringVar(&musicTitle, "title", "", "Track title")
	f.StringVar(&musicLyrics, "lyrics", "", "Lyrics (enables custom mode on suno)")
	f.BoolVar(&musicInstrumental, "instrumental", false, "Instrumental only (no vocals)")
	f.IntVarP(&musicDuration, "duration", "d", 0, "Duration in seconds")
	f.StringVar(&musicFormat, "format", "", "Audio format (suno backend)")
	f.StringVar(&musicJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	f.BoolVar(&musicDryRun, "dry-run", false, "Print request without calling API")

	musicCmd.AddCommand(musicGenerateCmd)
	musicCmd.AddCommand(musicQueryCmd)

	// Silence usage on all subcommands — errors are runtime API failures.
	for _, sub := range musicCmd.Commands() {
		sub.SilenceUsage = true
	}

	rootCmd.AddCommand(musicCmd)
}
