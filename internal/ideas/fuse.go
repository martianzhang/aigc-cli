package ideas

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// SearchOnline queries every source concurrently and merges their ranked
// lists round-robin (rank 1 of each source, then rank 2, ...), de-duplicated
// by entry identity. One error per failed source is returned; successful
// sources keep contributing results.
func SearchOnline(ctx context.Context, sources []Source, query string, limit int) ([]IdeaEntry, []error) {
	type outcome struct {
		entries []IdeaEntry
		err     error
	}

	outcomes := make([]outcome, len(sources))
	var wg sync.WaitGroup
	for i, src := range sources {
		wg.Add(1)
		go func(i int, src Source) {
			defer wg.Done()
			entries, err := src.Search(ctx, query, limit)
			if err != nil {
				outcomes[i] = outcome{err: fmt.Errorf("source %s: %w", src.Name(), err)}
				return
			}
			outcomes[i] = outcome{entries: entries}
		}(i, src)
	}
	wg.Wait()

	var errs []error
	lists := make([][]IdeaEntry, 0, len(outcomes))
	for _, o := range outcomes {
		if o.err != nil {
			errs = append(errs, o.err)
			continue
		}
		if len(o.entries) > 0 {
			lists = append(lists, o.entries)
		}
	}
	return mergeRoundRobin(lists), errs
}

// FuseRRF merges any number of ranked lists with Reciprocal Rank Fusion.
// Entries are keyed by source URL when present, otherwise by prompt text.
// Equal scores keep first-seen order so the output is deterministic.
func FuseRRF(lists [][]IdeaEntry, limit int) []SearchResult {
	type fused struct {
		entry IdeaEntry
		score float64
		order int
	}

	merged := make([]*fused, 0)
	index := make(map[string]int)
	for _, list := range lists {
		for rank, entry := range list {
			key := identityKey(entry)
			pos, ok := index[key]
			if !ok {
				merged = append(merged, &fused{entry: entry, order: len(merged)})
				pos = len(merged) - 1
				index[key] = pos
			}
			merged[pos].score += 1.0 / (rrfK + float64(rank+1))
		}
	}

	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].score != merged[j].score {
			return merged[i].score > merged[j].score
		}
		return merged[i].order < merged[j].order
	})
	if limit > 0 && limit < len(merged) {
		merged = merged[:limit]
	}

	results := make([]SearchResult, 0, len(merged))
	for _, item := range merged {
		results = append(results, SearchResult{Entry: item.entry, Score: int(item.score * 1000)})
	}
	return results
}

// mergeRoundRobin interleaves ranked lists and drops duplicate entries.
func mergeRoundRobin(lists [][]IdeaEntry) []IdeaEntry {
	seen := make(map[string]bool)
	var merged []IdeaEntry
	for rank := 0; ; rank++ {
		advanced := false
		for _, list := range lists {
			if rank >= len(list) {
				continue
			}
			advanced = true
			entry := list[rank]
			key := identityKey(entry)
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, entry)
		}
		if !advanced {
			return merged
		}
	}
}

// identityKey is the deduplication key shared by SearchOnline and FuseRRF.
func identityKey(entry IdeaEntry) string {
	if entry.SourceURL != "" {
		return "url:" + entry.SourceURL
	}
	return "prompt:" + entry.Prompt
}
