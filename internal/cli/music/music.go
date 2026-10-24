package music

import (
	"sync"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
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
		p := options.Shared.ResolveProvider(options.ProviderNameMusic)
		musicClientInst = client.NewFromProvider(p)
		options.ApplyTimeout(musicClientInst, "music", client.MusicTimeout)
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
	musicInjectJSON   string
	musicDryRun       bool
)

// ============================================================================
// Parent command: aigc-cli music
// ============================================================================
var musicCmd = &cobra.Command{
	Use:          "music",
	Short:        "AI music generation (APIMart suno/flowmusic, OpenRouter Lyria-3, Bailian Fun-Music)",
	SilenceUsage: true,
	Long: `Generate music from a natural-language prompt.

The backend is selected by Provider:
  APIMart (default)  async: submit -> poll -> download
    --model suno (default) — prompt/lyrics/style/title, optional duration/format
    --model flowmusic      — sound_prompt/lyrics/title/length
  OpenRouter         synchronous streaming (Google Lyria-3)
    --provider openrouter --model google/lyria-3-clip-preview (or -pro-preview)
  Bailian (阿里云百炼) synchronous (Fun-Music)
    --provider dashscope --model fun-music-v1 (or fun-music-preview)

Provider-private fields are not flags: pass them with --inject-json, which
merges into the request body. Flags marked with a backend name in --help
only take effect on that backend.

Subcommands:
  generate (gen)   Submit a music generation task
  query            Get an APIMart music task status and result

Examples:
  aigc-cli music generate --prompt "city pop"
  aigc-cli music gen --prompt "rock" --model flowmusic
  aigc-cli music gen --provider openrouter --model google/lyria-3-clip-preview --prompt "ambient"
  aigc-cli music gen --provider dashscope --model fun-music-v1 --prompt "夏日清新民谣"
  aigc-cli music query task_xxx`,
}

// ============================================================================
// Init — register subcommands and flags
// ============================================================================
func init() {
	f := musicGenerateCmd.Flags()
	f.StringVarP(&musicPrompt, "prompt", "p", "", "Music prompt / description (or style; an existing file path is read)")
	f.StringVarP(&musicModel, "model", "m", "", "Backend model: suno (default), flowmusic, or fun-music-v1/-preview")
	f.StringVar(&musicStyle, "style", "", "Style (suno/flowmusic style field; fallback for prompt)")
	f.StringVar(&musicTitle, "title", "", "Track title (suno/flowmusic)")
	f.StringVar(&musicLyrics, "lyrics", "", "Lyrics (suno/flowmusic; enables custom mode on suno)")
	f.BoolVar(&musicInstrumental, "instrumental", false, "Instrumental only, no vocals (suno/flowmusic; Fun-Music does not support it)")
	f.IntVarP(&musicDuration, "duration", "d", 0, "Duration in seconds (suno/flowmusic; Fun-Music does not support it)")
	f.StringVar(&musicFormat, "format", "", "Audio format (suno/OpenRouter/百炼; flowmusic does not support it)")
	f.StringVar(&musicJSONInput, "json", "", "JSON file path, JSON string, or \"-\" for stdin")
	f.StringVar(&musicInjectJSON, "inject-json", "", "JSON object merged additively into the request body (arrays append, scalars overwrite); use it for vendor-private fields; JSON string or file path")

	musicCmd.AddCommand(musicGenerateCmd)
	musicCmd.AddCommand(musicQueryCmd)

	// --dry-run is shared by all music subcommands.
	musicCmd.PersistentFlags().BoolVar(&musicDryRun, "dry-run", false, "Print request without calling API")

	// Silence usage on all subcommands — errors are runtime API failures.
	for _, sub := range musicCmd.Commands() {
		sub.SilenceUsage = true
	}

}

// Cmd returns the music command tree.
func Cmd() *cobra.Command { return musicCmd }
