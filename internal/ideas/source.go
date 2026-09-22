package ideas

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Source names accepted by --source. SourceAll is a selector, not a backend.
const (
	SourceLocal           = "local"
	SourceAIPromptLibrary = "aipromptslibrary"
	SourcePromptsChat     = "prompts.chat"
	SourceOpenArt         = "openart"
	SourceAll             = "all"
)

// errEmptyQuery is returned when a source is asked to search without a query.
var errEmptyQuery = errors.New("empty query")

// Source is a prompt retrieval backend (local dataset or an online library).
type Source interface {
	Name() string
	Search(ctx context.Context, query string, limit int) ([]IdeaEntry, error)
}

// AllSourceNames lists every accepted --source value, including the "all" selector.
func AllSourceNames() []string {
	return []string{SourceAll, SourceLocal, SourceAIPromptLibrary, SourcePromptsChat, SourceOpenArt}
}

// OnlineSourceNames lists the online libraries in their default order.
func OnlineSourceNames() []string {
	return []string{SourceAIPromptLibrary, SourcePromptsChat, SourceOpenArt}
}

// ResolveSources turns --source specs into an ordered source list.
//
// An empty spec list or any "all" element selects the local dataset (only when
// hasLocal) followed by every online source. Individual names may be
// comma-separated inside one element, are matched case-insensitively, keep the
// order given, and are de-duplicated. Requesting "local" without hasLocal is an
// error; unknown names are rejected.
func ResolveSources(specs []string, hasLocal bool) ([]Source, error) {
	names := normalizeSpecs(specs)
	if len(names) == 0 {
		names = []string{SourceAll}
	}
	for _, name := range names {
		if name == SourceAll {
			return allSources(hasLocal), nil
		}
	}

	seen := make(map[string]bool, len(names))
	sources := make([]Source, 0, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		if name == SourceLocal {
			if !hasLocal {
				return nil, fmt.Errorf("local source requested but ideas.json was not found.\n  Run 'aigc-cli ideas init' to download the prompt dataset")
			}
			sources = append(sources, localSource{})
			continue
		}
		if !isOnlineSourceName(name) {
			return nil, fmt.Errorf("unknown source %q: valid sources are %s", name, strings.Join(AllSourceNames(), ", "))
		}
		sources = append(sources, onlineSource(name))
	}
	return sources, nil
}

// HasLocal reports whether the resolved sources include the local dataset.
func HasLocal(sources []Source) bool {
	for _, src := range sources {
		if src.Name() == SourceLocal {
			return true
		}
	}
	return false
}

// OnlineSources filters the resolved sources down to the online libraries.
func OnlineSources(sources []Source) []Source {
	online := make([]Source, 0, len(sources))
	for _, src := range sources {
		if src.Name() != SourceLocal {
			online = append(online, src)
		}
	}
	return online
}

// normalizeSpecs splits comma-separated specs, trims, lowercases and drops empties.
func normalizeSpecs(specs []string) []string {
	var names []string
	for _, spec := range specs {
		for _, part := range strings.Split(spec, ",") {
			if name := strings.ToLower(strings.TrimSpace(part)); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// allSources expands the "all" selector.
func allSources(hasLocal bool) []Source {
	sources := make([]Source, 0, len(OnlineSourceNames())+1)
	if hasLocal {
		sources = append(sources, localSource{})
	}
	for _, name := range OnlineSourceNames() {
		sources = append(sources, onlineSource(name))
	}
	return sources
}

// onlineSource builds the provider for an online source name.
func onlineSource(name string) Source {
	switch name {
	case SourceAIPromptLibrary:
		return aiPromptLibrarySource{}
	case SourcePromptsChat:
		return promptsChatSource{}
	case SourceOpenArt:
		return openArtSource{}
	default:
		return nil
	}
}

// isOnlineSourceName reports whether name is one of the online libraries.
func isOnlineSourceName(name string) bool {
	for _, candidate := range OnlineSourceNames() {
		if candidate == name {
			return true
		}
	}
	return false
}

// localSource searches the local ideas.json dataset at its default location.
type localSource struct{}

// Name returns the source name.
func (localSource) Name() string { return SourceLocal }

// Search ranks local dataset entries with the BM25 index.
func (localSource) Search(_ context.Context, query string, limit int) ([]IdeaEntry, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errEmptyQuery
	}
	entries, err := LoadIdeas("")
	if err != nil {
		return nil, err
	}
	results := SearchIdeas(entries, BuildBM25Index(entries), query)
	if limit > 0 && limit < len(results) {
		results = results[:limit]
	}
	ranked := make([]IdeaEntry, 0, len(results))
	for _, result := range results {
		ranked = append(ranked, result.Entry)
	}
	return ranked, nil
}
