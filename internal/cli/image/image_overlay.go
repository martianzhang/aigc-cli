package image

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/cli/options"
)

// imageFlagOverlay returns the JSON body keys to overwrite for the image flags
// the user explicitly set, so CLI flags beat a --json body. Behavioral flags
// (--dry-run, --preview, --decode, --edit, --mode, --save-prompt, --json,
// --compress, --provider/--api-key/--api-base, --output, --verbose, --timeout,
// --config, --print-config) are absent by design: they control execution, never
// the wire body.
func imageFlagOverlay(cmd *cobra.Command) (map[string]any, error) {
	set := map[string]any{}
	f := cmd.Flags()

	if f.Changed("prompt") {
		prompt, err := resolvePrompt()
		if err != nil {
			return nil, err
		}
		set["prompt"] = prompt
	}

	stringFlags := []struct {
		flag string
		key  string
		val  string
	}{
		{"size", "size", genSize},
		{"ratio", "ratio", genRatio},
		{"resolution", "resolution", genResolution},
		{"quality", "quality", genQuality},
		{"background", "background", genBackground},
		{"moderation", "moderation", genModeration},
		{"output-format", "output_format", genOutputFormat},
		{"mask-url", "mask_url", genMaskURL},
		{"style", "style", genStyle},
		{"response-format", "response_format", genResponseFmt},
	}
	for _, sf := range stringFlags {
		if f.Changed(sf.flag) {
			set[sf.key] = sf.val
		}
	}

	if f.Changed("output-compression") {
		set["output_compression"] = genCompression
	}
	if f.Changed("n") {
		set["n"] = genN
	}
	if f.Changed("image-url") {
		urls, err := f.GetStringArray("image-url")
		if err != nil {
			return nil, fmt.Errorf("failed to read --image-url: %w", err)
		}
		set["image_urls"] = urls
	}
	if options.HasFlagChanged(cmd, "model") {
		set["model"] = options.Shared.Model
	}

	return set, nil
}
