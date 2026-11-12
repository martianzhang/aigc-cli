// Package ideas implements the `aigc-cli ideas` command.
package ideas

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/ideas"
	"github.com/martianzhang/aigc-cli/internal/options"
	"github.com/martianzhang/aigc-cli/internal/types"
	"github.com/martianzhang/aigc-cli/internal/ui"
)

// Deps carries the runtime configuration the ideas command needs.
type Deps struct {
	Cfg       *types.Config
	OutputDir string
	Verbose   bool
}

// defaultLimit is the number of results shown when --limit is not set.
const defaultLimit = 8

type cmdFlags struct {
	limit      int
	random     bool
	save       bool
	preview    bool
	findImage  string
	sources    []string
	id         string
	promptOnly bool
	// limitSet records whether --limit was explicitly passed, so the
	// no-keyword random path can keep the "one random idea" default without
	// overriding an explicit --limit (e.g. --random --limit 3).
	limitSet bool
}

// NewCommand builds the `ideas` command tree. deps is resolved at run time.
func NewCommand(deps func() Deps) *cobra.Command {
	var f cmdFlags

	cmd := &cobra.Command{
		Use:          "ideas [keywords]",
		Aliases:      []string{"idea"},
		Short:        "Search AI image prompt ideas (also: idea)",
		SilenceUsage: true,
		Long: `Search AI image generation prompt ideas from a local ideas.json file
and/or online prompt libraries.

Outputs markdown by default, with each result containing an idea ID,
reference images, full prompt text, and metadata.

Use --prompt-only to print just the prompt text, so it can be piped into
"aigc-cli image -p". Use --id to fetch a previously seen idea again by its ID.

Keywords can be passed as arguments or via stdin.

Data file: ~/.config/aigc-cli/ideas.json (run "aigc-cli ideas init" to download).

Sources (--source):
  all               Local dataset plus every online source (default)
  local             Local ideas.json dataset only
  aipromptslibrary  aipromptslibrary.sh image-generation prompts
  prompts.chat      prompts.chat community prompt library
  openart           openart.ai community prompts (experimental)
  civitai           civitai.com image prompts (matched via model search)
  youmind           youmind.com prompt library (image + video + webpage)

Pass one value, a comma-separated list, or repeat the flag:
  --source local,openart
  --source prompts.chat --source openart

Online sources are keyless but need network access; the configured
http_proxy is respected. When ideas.json is absent, "all" searches the
online sources only.`,
		Example: `  aigc-cli ideas "cinematic portrait"
  aigc-cli ideas "cyberpunk city" --source aipromptslibrary
  aigc-cli ideas "portrait" --source prompts.chat --source openart
  aigc-cli ideas "cyberpunk city" --source civitai
  aigc-cli ideas "luxury perfume" --limit 3
  aigc-cli ideas --random --limit 1              # single random idea
  aigc-cli ideas --random --limit 1 --prompt-only
  aigc-cli ideas --id 1a2b3c4d5e6f --prompt-only
  aigc-cli image -p "$(aigc-cli ideas --id 1a2b3c4d5e6f --prompt-only)"
  echo "cyberpunk city" | aigc-cli ideas`,
		RunE: func(cmd *cobra.Command, args []string) error {
			f.limitSet = cmd.Flags().Changed("limit")
			return run(deps(), args, &f)
		},
	}

	fl := cmd.Flags()
	fl.IntVarP(&f.limit, "limit", "l", defaultLimit, "Number of results to show (default 8; 1 for a bare random idea)")
	fl.BoolVar(&f.random, "random", false, "Shuffle matched results randomly (default: ranked by relevance)")
	fl.BoolVar(&f.save, "save", false, "Download reference images to local directory")
	fl.BoolVar(&f.preview, "preview", false, "Open saved images with system default viewer (implies --save)")
	fl.StringVar(&f.findImage, "find-image", "", "Search by image filename (matches image_urls in dataset)")
	fl.StringVar(&f.id, "id", "", "Show one idea by its ID (IDs are printed with each result; local dataset only)")
	fl.BoolVar(&f.promptOnly, "prompt-only", false, "Print only prompt text, no titles/images/metadata (for piping into aigc-cli image -p)")
	fl.StringSliceVar(&f.sources, "source", []string{ideas.SourceAll}, "Sources to search (comma-separated or repeatable): all, local, aipromptslibrary, prompts.chat, openart, civitai, youmind")

	cmd.AddCommand(newInitCommand(deps))
	return cmd
}

func run(d Deps, args []string, f *cmdFlags) error {
	if f.id != "" {
		return runByID(d, f)
	}

	keywords, err := resolveKeywords(args)
	if err != nil {
		return err
	}

	dataPath := resolveDataPath(d.Cfg)
	sources, err := ideas.ResolveSources(f.sources, dataPath != "")
	if err != nil {
		return err
	}

	var (
		results      []ideas.SearchResult
		usedSemantic bool
		embProvider  string
		embModel     string
		meta         string
	)
	switch {
	case f.findImage != "":
		entries, err := ideas.LoadIdeas(dataPath)
		if err != nil {
			return err
		}
		results = ideas.SearchByImage(entries, f.findImage)
		keywords = "image: " + f.findImage
	case keywords == "":
		// No query: keep the random local behaviour; online sources need a query.
		entries, err := ideas.LoadIdeas(dataPath)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return emptyResult(f, "ideas.json is empty. Run `aigc-cli ideas init` to download.")
		}
		f.random = true
		if !f.limitSet {
			f.limit = 1
		}
		for i := range entries {
			results = append(results, ideas.SearchResult{Entry: entries[i]})
		}
		keywords = "random ideas"
	default:
		var lists [][]ideas.IdeaEntry
		if ideas.HasLocal(sources) {
			ic := ideasConfig(d.Cfg)
			embedder, embErr := options.BuildIdeasEmbedder(ic)
			if embErr != nil {
				ui.Eprintf("Warning: %v\n", embErr)
			}
			if embedder != nil {
				usedSemantic = true
				if ic != nil {
					embProvider, embModel = ic.EmbeddingProvider, ic.EmbeddingModel
				}
			}
			local, empty, err := localResults(dataPath, keywords, embedder, ic)
			if err != nil {
				return err
			}
			if empty && len(sources) == 1 {
				return emptyResult(f, "ideas.json is empty. Run `aigc-cli ideas init` to download.")
			}
			if len(local) > 0 {
				lists = append(lists, local)
			}
		}
		if merged := searchOnlineSources(sources, keywords, f.limit, d.Verbose); len(merged) > 0 {
			lists = append(lists, merged)
		}
		results = ideas.FuseRRF(lists, 0)
		meta = retrievalMeta(sources, usedSemantic, embProvider, embModel)
	}
	if len(results) == 0 {
		return emptyResult(f, "No matching prompts found.")
	}

	total := len(results)

	if f.random {
		rand.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})
	}

	limit := min(f.limit, total)
	results = results[:limit]

	if f.promptOnly {
		printPrompts(results)
		return nil
	}

	if f.preview && !f.save {
		f.save = true
	}

	if f.save {
		var imgEntries []ideas.IdeaEntry
		for _, r := range results {
			imgEntries = append(imgEntries, r.Entry)
		}
		saved, _ := saveIdeaImages(imgEntries, d.OutputDir)
		return outputMarkdown(results, keywords, total, saved, f.preview, meta)
	}

	return outputMarkdown(results, keywords, total, nil, f.preview, meta)
}

// runByID resolves a single idea from the local dataset by its stable ID.
// Online results are ephemeral, so only local dataset entries can be looked up.
func runByID(d Deps, f *cmdFlags) error {
	entries, err := ideas.LoadIdeas(resolveDataPath(d.Cfg))
	if err != nil {
		return err
	}

	want := strings.TrimSpace(f.id)
	for i := range entries {
		if !strings.EqualFold(entries[i].StableID(), want) {
			continue
		}
		results := []ideas.SearchResult{{Entry: entries[i]}}
		if f.promptOnly {
			printPrompts(results)
			return nil
		}
		return outputMarkdown(results, "id: "+want, 1, nil, false, "")
	}

	return fmt.Errorf("idea %q not found in the local dataset (IDs are printed with each result; only local dataset ideas can be looked up)", want)
}

// emptyResult reports an empty result set. With --prompt-only the message
// becomes an error, so a human-readable notice can never be piped into
// `aigc-cli image -p` as if it were a prompt.
func emptyResult(f *cmdFlags, message string) error {
	if f.promptOnly {
		return errors.New(message)
	}
	ui.Println(message)
	return nil
}

// printPrompts writes one prompt per result, with no markdown or metadata, so
// the output can be piped straight into `aigc-cli image -p`.
func printPrompts(results []ideas.SearchResult) {
	for _, r := range results {
		ui.Println(strings.TrimSpace(r.Entry.DisplayPrompt()))
	}
}
