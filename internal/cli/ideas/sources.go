package ideas

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/martianzhang/aigc-cli/internal/ideas"
	"github.com/martianzhang/aigc-cli/internal/knowledge"
)

// onlineSearchTimeout bounds the concurrent online source queries. It must
// cover the slowest source: most issue a single request, but civitai resolves
// models first and only then fetches their images (two round-trips).
const onlineSearchTimeout = 12 * time.Second

// localResults builds the ranked local list, fusing keyword (BM25) and semantic
// (embedding) ranks when an embedder is configured. The empty flag reports a
// present-but-empty dataset so the caller can keep the historical message.
func localResults(dataPath, keywords string, embedder knowledge.Embedder) ([]ideas.IdeaEntry, bool, error) {
	entries, err := ideas.LoadIdeas(dataPath)
	if err != nil {
		return nil, false, err
	}
	if len(entries) == 0 {
		return nil, true, nil
	}

	ranked := ideas.SearchIdeas(entries, ideas.BuildBM25Index(entries), keywords)
	bm25List := make([]ideas.IdeaEntry, 0, len(ranked))
	for _, r := range ranked {
		bm25List = append(bm25List, r.Entry)
	}

	if embedder == nil {
		return bm25List, false, nil
	}
	vectors, err := ideas.LoadOrBuildEmbeddings(entries, embedder, dataPath, ideasProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: ideas semantic search disabled: %v\n", err)
		return bm25List, false, nil
	}
	semanticList, err := ideas.SemanticEntries(entries, vectors, embedder, keywords, ideas.SemanticTopK)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: ideas semantic search disabled: %v\n", err)
		return bm25List, false, nil
	}

	fused := ideas.FuseRRF([][]ideas.IdeaEntry{bm25List, semanticList}, 0)
	list := make([]ideas.IdeaEntry, len(fused))
	for i, r := range fused {
		list[i] = r.Entry
	}
	return list, false, nil
}

// ideasProgress reports embedding build progress (only called on a cache miss).
func ideasProgress(done, total int) {
	if done == total || done%1024 == 0 {
		fmt.Fprintf(os.Stderr, "  Building ideas embeddings: %d/%d\n", done, total)
	}
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
