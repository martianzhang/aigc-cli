// Package ideas implements the `aigc-cli ideas` command.
package ideas

import (
	"fmt"
	"math/rand"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/ideas"
	"github.com/martianzhang/aigc-cli/internal/types"
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
	limit     int
	random    bool
	jsonOut   bool
	save      bool
	preview   bool
	findImage string
	sources   []string
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

Outputs markdown by default, with each result containing
reference images, full prompt text, and metadata.

Keywords can be passed as arguments or via stdin.

Data file: ~/.config/aigc-cli/ideas.json (run "aigc-cli ideas init" to download).

Sources (--source):
  all               Local dataset plus every online source (default)
  local             Local ideas.json dataset only
  aipromptslibrary  aipromptslibrary.sh image-generation prompts
  prompts.chat      prompts.chat community prompt library
  openart           openart.ai community prompts (experimental)
  civitai           civitai.com image prompts (matched via model search)

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
  echo "cyberpunk city" | aigc-cli ideas
  aigc-cli ideas --json "cat" | jq '.results[].prompt'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(deps(), args, &f)
		},
	}

	fl := cmd.Flags()
	fl.IntVarP(&f.limit, "limit", "l", defaultLimit, "Number of results to show (default 8)")
	fl.BoolVar(&f.random, "random", false, "Shuffle matched results randomly (default: ranked by relevance)")
	fl.BoolVar(&f.jsonOut, "json", false, "Output as JSON instead of markdown")
	fl.BoolVar(&f.save, "save", false, "Download reference images to local directory")
	fl.BoolVar(&f.preview, "preview", false, "Open saved images with system default viewer (implies --save)")
	fl.StringVar(&f.findImage, "find-image", "", "Search by image filename (matches image_urls in dataset)")
	fl.StringSliceVar(&f.sources, "source", []string{ideas.SourceAll}, "Sources to search (comma-separated or repeatable): all, local, aipromptslibrary, prompts.chat, openart, civitai")

	cmd.AddCommand(newInitCommand(deps))
	return cmd
}

func run(d Deps, args []string, f *cmdFlags) error {
	keywords, err := resolveKeywords(args)
	if err != nil {
		return err
	}

	dataPath := resolveDataPath(d.Cfg)
	sources, err := ideas.ResolveSources(f.sources, dataPath != "")
	if err != nil {
		return err
	}

	var results []ideas.SearchResult
	switch {
	case f.findImage != "":
		entries, err := ideas.LoadIdeas(dataPath)
		if err != nil {
			return err
		}
		results = ideas.SearchByImage(entries, f.findImage)
		keywords = "图片: " + f.findImage
	case keywords == "":
		// No query: keep the random local behaviour; online sources need a query.
		entries, err := ideas.LoadIdeas(dataPath)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("ideas.json is empty. Run `aigc-cli ideas init` to download.")
			return nil
		}
		f.random = true
		f.limit = 1
		for i := range entries {
			results = append(results, ideas.SearchResult{Entry: entries[i]})
		}
		keywords = "随机灵感"
	default:
		var lists [][]ideas.IdeaEntry
		if ideas.HasLocal(sources) {
			local, empty, err := localResults(dataPath, keywords)
			if err != nil {
				return err
			}
			if empty && len(sources) == 1 {
				fmt.Println("ideas.json is empty. Run `aigc-cli ideas init` to download.")
				return nil
			}
			if len(local) > 0 {
				lists = append(lists, local)
			}
		}
		if merged := searchOnlineSources(sources, keywords, f.limit, d.Verbose); len(merged) > 0 {
			lists = append(lists, merged)
		}
		results = ideas.FuseRRF(lists, 0)
	}
	if len(results) == 0 {
		fmt.Println("没有找到匹配的提示词。")
		return nil
	}

	total := len(results)

	if f.random {
		rand.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})
	}

	limit := f.limit
	if limit > total {
		limit = total
	}
	results = results[:limit]

	if f.preview && !f.save {
		f.save = true
	}

	if f.save {
		var imgEntries []ideas.IdeaEntry
		for _, r := range results {
			imgEntries = append(imgEntries, r.Entry)
		}
		saved, _ := saveIdeaImages(imgEntries, d.OutputDir)
		if f.jsonOut {
			return outputJSON(results, total)
		}
		return outputMarkdown(results, keywords, total, saved, f.preview)
	}

	if f.jsonOut {
		return outputJSON(results, total)
	}
	return outputMarkdown(results, keywords, total, nil, f.preview)
}
