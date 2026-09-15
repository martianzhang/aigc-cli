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
	f.StringVarP(&musicPrompt, "prompt", "p", "", "Music prompt / description (or style)")
	f.StringVarP(&musicModel, "model", "m", "", "Backend model: suno (default), flowmusic, or fun-music-v1/-preview")
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

}

// Cmd returns the music command tree.
func Cmd() *cobra.Command { return musicCmd }
