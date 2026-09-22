package ideas

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/martianzhang/aigc-cli/internal/ideas"
)

// onlineSearchTimeout bounds the concurrent online source queries.
const onlineSearchTimeout = 5 * time.Second

// localResults builds the ranked local list. The empty flag reports a
// present-but-empty dataset so the caller can keep the historical message.
func localResults(dataPath, keywords string) ([]ideas.IdeaEntry, bool, error) {
	entries, err := ideas.LoadIdeas(dataPath)
	if err != nil {
		return nil, false, err
	}
	if len(entries) == 0 {
		return nil, true, nil
	}
	ranked := ideas.SearchIdeas(entries, ideas.BuildBM25Index(entries), keywords)
	list := make([]ideas.IdeaEntry, 0, len(ranked))
	for _, r := range ranked {
		list = append(list, r.Entry)
	}
	return list, false, nil
}

// searchOnlineSources queries every online source. Source failures are reported
// only when verbose is set.
func searchOnlineSources(sources []ideas.Source, keywords string, limit int, verbose bool) []ideas.IdeaEntry {
	online := ideas.OnlineSources(sources)
	if len(online) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), onlineSearchTimeout)
	defer cancel()

	merged, errs := ideas.SearchOnline(ctx, online, keywords, limit)
	if verbose {
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}
	return merged
}
