package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/martianzhang/apimart-cli/internal/service"
)

// --- data structures ---

// IdeaEntry represents a single prompt entry in ideas.json.
type IdeaEntry struct {
	Title     string   `json:"title,omitempty"`
	TitleZh   string   `json:"title_zh,omitempty"`
	Prompt    string   `json:"prompt"`
	PromptZh  string   `json:"prompt_zh,omitempty"`
	ImageURLs []string `json:"image_urls,omitempty"`
	SourceURL string   `json:"source_url,omitempty"`
	Author    string   `json:"author,omitempty"`
	License   string   `json:"license,omitempty"`
	Lang      string   `json:"lang"`
}

// searchResult pairs an entry with its relevance score.
type searchResult struct {
	entry IdeaEntry
	score int
}

// ideas flag variables
var (
	ideasLimit      int
	ideasRandom     bool
	ideasJSON       bool
	ideasSaveImages bool
	ideasPreview    bool
	ideasFindImage  string
)

const ideasDefaultLimit = 8

// ideasCmd represents the `apimart-cli ideas` command.
var ideasCmd = &cobra.Command{
	Use:          "ideas [keywords]",
	Short:        "Search AI image prompt ideas from local ideas.json",
	SilenceUsage: true,
	Long: `Search AI image generation prompt ideas from a local ideas.json file.

Outputs markdown by default, with each result containing
reference images, full prompt text, and metadata.

Keywords can be passed as arguments or via stdin:

  apimart-cli ideas "cinematic portrait"
  apimart-cli ideas "luxury perfume" --limit 3
  apimart-cli ideas --random              # random ideas without keywords
  apimart-cli ideas --random --limit 1    # single random idea
  echo "cyberpunk city" | apimart-cli ideas
  apimart-cli ideas --json "cat" | jq '.results[].prompt'

Data file: ~/.config/apimart/ideas.json (run "apimart-cli ideas init" to download).`,
	RunE: runIdeas,
}

// searchIdeasText searches the local ideas database and returns formatted results.
// Shared by CLI and agent loop.
func searchIdeasText(keywords string, limit int) (string, error) {
	entries, rawData, err := loadIdeas()
	if err != nil {
		return "", fmt.Errorf("failed to load ideas: %w", err)
	}

	if len(entries) == 0 {
		return "No ideas found in the database. Run `apimart-cli ideas init` to download.", nil
	}

	// Load or build BM25 index
	hash := computeHash(rawData)
	idx := loadCachedIndex(shared.Cfg, hash)
	if idx == nil {
		idx = buildBM25Index(entries)
		saveCachedIndex(shared.Cfg, idx, hash)
	}

	results := searchIdeas(entries, idx, keywords)
	if len(results) == 0 {
		return "No matching prompts found.", nil
	}

	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	results = results[:limit]

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d result(s) for \"%s\":\n\n", len(results), keywords)
	for i, r := range results {
		fmt.Fprintf(&b, "%d. %s\n", i+1, r.entry.Prompt)
		if r.entry.Title != "" {
			fmt.Fprintf(&b, "   Title: %s\n", r.entry.Title)
		}
	}
	return b.String(), nil
}

func runIdeas(cmd *cobra.Command, args []string) error {
	// Resolve keywords
	keywords, err := resolveIdeasKeywords(args)
	if err != nil {
		return err
	}

	// Load ideas.json (from external file)
	entries, rawData, err := loadIdeas()
	if err != nil {
		return err
	}

	// Load or build BM25 index (cache-supported)
	var idx *bm25Index
	if keywords != "" {
		hash := computeHash(rawData)
		idx = loadCachedIndex(shared.Cfg, hash)
		if idx == nil {
			idx = buildBM25Index(entries)
			saveCachedIndex(shared.Cfg, idx, hash)
		}
	}

	// Search
	var results []searchResult
	if ideasFindImage != "" {
		results = searchByImage(entries, ideasFindImage)
		keywords = "图片: " + ideasFindImage
	} else if keywords != "" {
		results = searchIdeas(entries, idx, keywords)
	} else if ideasRandom {
		// --random without keywords: return all entries randomly
		for i := range entries {
			results = append(results, searchResult{entry: entries[i]})
		}
		keywords = "随机灵感"
	} else {
		return fmt.Errorf("keywords or --find-image are required")
	}
	if len(results) == 0 {
		fmt.Println("没有找到匹配的提示词。")
		return nil
	}

	total := len(results)

	// Randomize if --random flag is set (shuffles matched results)
	if ideasRandom {
		rand.Shuffle(len(results), func(i, j int) {
			results[i], results[j] = results[j], results[i]
		})
	}

	// Apply limit
	limit := ideasLimit
	if limit > total {
		limit = total
	}
	results = results[:limit]

	// --preview implies --save: system viewer needs files on disk
	if ideasPreview && !ideasSaveImages {
		ideasSaveImages = true
	}

	// Save images if requested
	if ideasSaveImages {
		var entries []IdeaEntry
		for _, r := range results {
			entries = append(entries, r.entry)
		}
		saved, _ := saveIdeaImages(entries)
		if ideasJSON {
			return outputJSON(results, total)
		}
		return outputMarkdown(results, keywords, total, saved)
	}

	// Output
	if ideasJSON {
		return outputJSON(results, total)
	}
	return outputMarkdown(results, keywords, total, nil)
}

// --- keyword resolution ---

func resolveIdeasKeywords(args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", nil
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// --- data loading ---

func loadIdeas() (entries []IdeaEntry, rawData []byte, err error) {
	path := resolveIdeasDataPath(shared.Cfg)
	if path == "" {
		return nil, nil, fmt.Errorf("ideas.json not found.\n  Run 'apimart-cli ideas init' to download the prompt dataset,\n  or place ideas.json at ~/.config/apimart/ideas.json")
	}
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, nil, fmt.Errorf("invalid ideas.json: %w", err)
	}
	return entries, data, nil
}

// --- BM25 + n-gram hybrid search ---

const (
	bm25K1 = 1.2  // BM25 term frequency saturation
	bm25B  = 0.75 // BM25 length normalization
	rrfK   = 30.0 // RRF fusion constant
)

// bm25Index holds precomputed corpus statistics and pre-tokenized data.
type bm25Index struct {
	avgDocLen float64
	docCount  int
	idf       map[string]float64
	entries   []IdeaEntry

	// Pre-tokenized/cached to avoid re-scanning on every query
	docTokens [][]string       // tokens per doc, for BM25 scoring
	docSet    []map[string]int // token→count per doc, for AND filtering + TF
	docTexts  []string         // pre-computed searchableText, for n-gram
}

// buildBM25Index walks all entries, tokenizes once (in parallel), and pre-computes everything.
func buildBM25Index(entries []IdeaEntry) *bm25Index {
	n := len(entries)
	idx := &bm25Index{
		docCount:  n,
		idf:       make(map[string]float64),
		entries:   entries,
		docTokens: make([][]string, n),
		docSet:    make([]map[string]int, n),
		docTexts:  make([]string, n),
	}

	// Pre-compute searchable text (lightweight, serial)
	for i, e := range entries {
		idx.docTexts[i] = searchableText(e)
	}

	// Parallel tokenization
	numWorkers := runtime.NumCPU()
	type workerResult struct {
		totalTokens int
		docFreq     map[string]int
	}

	work := make(chan int, n)
	var wg sync.WaitGroup
	results := make([]workerResult, numWorkers)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		wr := &results[w]
		wr.docFreq = make(map[string]int)

		go func() {
			defer wg.Done()
			for i := range work {
				terms := tokenize(idx.docTexts[i])
				idx.docTokens[i] = terms
				wr.totalTokens += len(terms)

				tf := make(map[string]int, len(terms)/2)
				seen := make(map[string]bool, len(terms)/2)
				for _, t := range terms {
					tf[t]++
					if !seen[t] {
						wr.docFreq[t]++
						seen[t] = true
					}
				}
				idx.docSet[i] = tf
			}
		}()
	}

	for i := 0; i < n; i++ {
		work <- i
	}
	close(work)
	wg.Wait()

	// Merge per-worker results
	totalTokens := 0
	docFreq := make(map[string]int)
	for w := 0; w < numWorkers; w++ {
		wr := results[w]
		totalTokens += wr.totalTokens
		for term, df := range wr.docFreq {
			docFreq[term] += df
		}
	}
	idx.avgDocLen = float64(totalTokens) / float64(max(n, 1))

	for term, df := range docFreq {
		fd := float64(df)
		fn := float64(n)
		idx.idf[term] = math.Log(1 + (fn-fd+0.5)/(fd+0.5))
	}

	return idx
}

// bm25Score returns the BM25 score for a single entry given query terms.
func (idx *bm25Index) bm25Score(entryIdx int, queryTerms []string) float64 {
	tf := idx.docSet[entryIdx]
	docLen := float64(len(idx.docTokens[entryIdx]))
	var score float64

	for _, qt := range queryTerms {
		idf := idx.idf[qt]
		if idf == 0 {
			continue
		}
		freq := float64(tf[qt])
		score += idf * freq * (bm25K1 + 1) / (freq + bm25K1*(1-bm25B+bm25B*docLen/idx.avgDocLen))
	}

	return score
}

// ngramSet returns the set of character n-grams for a string.
func ngramSet(s string, n int) map[string]int {
	s = strings.ToLower(s)
	grams := make(map[string]int)
	for i := 0; i <= len(s)-n; i++ {
		grams[s[i:i+n]]++
	}
	return grams
}

// cosineSimilarity computes cosine similarity between two n-gram frequency maps.
func cosineSimilarity(a, b map[string]int) float64 {
	var dot, normA, normB float64
	for k, v := range a {
		normA += float64(v * v)
		if bv, ok := b[k]; ok {
			dot += float64(v * bv)
		}
	}
	for _, v := range b {
		normB += float64(v * v)
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// searchableText combines all searchable fields into one string.
func searchableText(e IdeaEntry) string {
	return e.Title + " " + e.TitleZh + " " + e.Prompt + " " + e.PromptZh
}

// tokenize splits text into lowercase tokens.
// ASCII sequences get fast-path single tokens (min 2 chars).
// CJK sequences are split into overlapping 2-grams.
func tokenize(text string) []string {
	var tokens []string
	var buf []rune
	for _, r := range text {
		// ASCII letter/digit → fast path, no unicode table lookup
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			buf = append(buf, r)
		} else if r >= 'A' && r <= 'Z' {
			buf = append(buf, r+32) // inline lowercase
		} else if r < 0x80 {
			if len(buf) > 0 {
				tokens = append(tokens, splitTokens(buf)...)
				buf = buf[:0]
			}
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			buf = append(buf, unicode.ToLower(r))
		} else if len(buf) > 0 {
			tokens = append(tokens, splitTokens(buf)...)
			buf = buf[:0]
		}
	}
	if len(buf) > 0 {
		tokens = append(tokens, splitTokens(buf)...)
	}
	return tokens
}

// splitTokens converts a rune buffer into one or more tokens.
// CJK-only buffers longer than 2 are split into overlapping 2-grams.
func splitTokens(buf []rune) []string {
	if len(buf) < 2 {
		return nil
	}
	allCJK := true
	for _, r := range buf {
		if !unicode.Is(unicode.Han, r) && !unicode.Is(unicode.Hiragana, r) &&
			!unicode.Is(unicode.Katakana, r) && !unicode.Is(unicode.Hangul, r) {
			allCJK = false
			break
		}
	}
	if allCJK && len(buf) > 2 {
		grams := make([]string, 0, len(buf)-1)
		for i := 0; i < len(buf)-1; i++ {
			grams = append(grams, string(buf[i:i+2]))
		}
		return grams
	}
	return []string{string(buf)}
}

// containsWord checks if term appears as a whole word in text.
// Uses byte-level word boundary detection that works for both
// ASCII (space-separated) and CJK (no spaces between characters).
func containsWord(text, term string) bool {
	lower := strings.ToLower(text)
	t := strings.ToLower(strings.TrimSpace(term))
	if t == "" {
		return false
	}
	// Scan for term at every position with word-boundary check
	for i := 0; i <= len(lower)-len(t); i++ {
		if lower[i:i+len(t)] != t {
			continue
		}
		if i > 0 && isWordChar(lower[i-1]) {
			continue
		}
		if i+len(t) < len(lower) && isWordChar(lower[i+len(t)]) {
			continue
		}
		return true
	}
	return false
}

// isWordChar returns true for ASCII word characters (a-z, A-Z, 0-9).
// Non-ASCII bytes (CJK, punctuation) are NOT word chars, which means
// CJK characters are naturally treated as word boundaries.
func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// andFilter returns true if all query terms exist in the pre-tokenized doc set.
func andFilter(docSet map[string]int, terms []string) bool {
	for _, t := range terms {
		if _, ok := docSet[t]; !ok {
			return false
		}
	}
	return true
}

// --- search ---

func searchIdeas(entries []IdeaEntry, idx *bm25Index, query string) []searchResult {
	queryTerms := tokenize(query)
	if len(queryTerms) == 0 {
		return nil
	}

	// Phase 1: AND filter (via pre-tokenized set) + BM25 scoring
	type scored struct {
		entryIdx int
		bm25     float64
	}
	var candidates []scored

	for i := range entries {
		if !andFilter(idx.docSet[i], queryTerms) {
			continue
		}
		s := idx.bm25Score(i, queryTerms)
		// Title boost: multiply BM25 by 2 if any query term appears in the title
		titleBoost := false
		for _, t := range queryTerms {
			if containsWord(entries[i].Title+" "+entries[i].TitleZh, t) {
				titleBoost = true
				break
			}
		}
		if titleBoost {
			s *= 2
		}

		candidates = append(candidates, scored{entryIdx: i, bm25: s})
	}

	if len(candidates) == 0 {
		return nil
	}

	// Phase 2: n-gram cosine similarity (uses pre-computed docTexts)
	queryNgrams := ngramSet(query, 3)
	type ngramScored struct {
		entryIdx int
		cosine   float64
	}
	var ngramCandidates []ngramScored
	for _, c := range candidates {
		docNgrams := ngramSet(idx.docTexts[c.entryIdx], 3)
		cos := cosineSimilarity(queryNgrams, docNgrams)
		ngramCandidates = append(ngramCandidates, ngramScored{entryIdx: c.entryIdx, cosine: cos})
	}

	// Phase 3: RRF fusion
	// Sort candidates by BM25 descending for BM25 ranks
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].bm25 > candidates[j].bm25
	})
	bm25Ranks := make(map[int]int)
	for i, c := range candidates {
		bm25Ranks[c.entryIdx] = i + 1
	}

	// Sort by n-gram cosine descending for n-gram ranks
	sort.Slice(ngramCandidates, func(i, j int) bool {
		return ngramCandidates[i].cosine > ngramCandidates[j].cosine
	})
	ngramRanks := make(map[int]int)
	for i, c := range ngramCandidates {
		ngramRanks[c.entryIdx] = i + 1
	}

	// RRF: combine ranks
	rrf := make(map[int]float64)
	for _, c := range candidates {
		r1 := float64(bm25Ranks[c.entryIdx])
		r2 := float64(ngramRanks[c.entryIdx])
		rrf[c.entryIdx] = 1.0/(rrfK+r1) + 1.0/(rrfK+r2)
	}

	results := make([]searchResult, 0, len(candidates))
	for _, c := range candidates {
		results = append(results, searchResult{
			entry: entries[c.entryIdx],
			score: int(rrf[c.entryIdx] * 1000),
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	return results
}

// searchByImage finds entries whose image_urls contain the given filename.
func searchByImage(entries []IdeaEntry, filename string) []searchResult {
	fn := strings.ToLower(filename)
	seen := make(map[string]bool)
	var results []searchResult
	for _, e := range entries {
		for _, url := range e.ImageURLs {
			if strings.Contains(strings.ToLower(url), fn) {
				key := url
				if key == "" {
					key = e.SourceURL
				}
				if key == "" {
					key = e.Title + "|" + e.Prompt
				}
				if !seen[key] {
					seen[key] = true
					results = append(results, searchResult{entry: e, score: 1})
				}
				break
			}
		}
	}
	return results
}

// --- markdown output ---

func outputMarkdown(results []searchResult, keywords string, total int, savedFiles []string) error {
	now := time.Now().Format("2006-01-02")
	header := fmt.Sprintf("# Ideas: %s\n> 找到 %d 个结果 · %s", keywords, total, now)
	if total > len(results) {
		header += fmt.Sprintf(" · 显示 %d/%d", len(results), total)
	}
	fmt.Println(header)
	fmt.Println()

	for i, r := range results {
		if i > 0 {
			fmt.Println("---")
			fmt.Println()
		}
		e := r.entry
		title := e.Title
		if title == "" {
			title = fmt.Sprintf("Result %d", i+1)
		}
		fmt.Printf("## %s\n\n", title)

		// Prompt (prefer zh for zh entries)
		prompt := e.Prompt
		if e.Lang == "zh" && e.PromptZh != "" {
			prompt = e.PromptZh
		}
		fmt.Printf("**提示词：**\n```\n%s\n```\n\n", prompt)

		// Images
		if len(e.ImageURLs) > 0 {
			if ideasSaveImages {
				for j, url := range e.ImageURLs {
					localPath := localImagePath(url)
					// Only use local path if the file was actually saved
					if _, err := os.Stat(localPath); err == nil {
						fmt.Printf("![参考图 %d](%s)\n\n", j+1, localPath)
					} else {
						fmt.Printf("![参考图 %d](%s)\n\n", j+1, url)
					}
				}
			} else {
				for j, url := range e.ImageURLs {
					fmt.Printf("![参考图 %d](%s)\n\n", j+1, url)
				}
			}
		}

		// Inline preview: show saved images right after their entry
		if ideasPreview && len(savedFiles) > 0 {
			for range e.ImageURLs {
				if len(savedFiles) == 0 {
					break
				}
				f := savedFiles[0]
				savedFiles = savedFiles[1:]
				if e := service.PreviewFile(f); e != nil {
					fmt.Fprintf(os.Stderr, "Warning: preview failed: %v\n", e)
				}
			}
		}

		// Metadata
		var meta []string
		if e.Author != "" {
			meta = append(meta, "作者: "+e.Author)
		}
		if e.SourceURL != "" {
			meta = append(meta, fmt.Sprintf("[来源](%s)", e.SourceURL))
		}
		if e.License != "" {
			meta = append(meta, e.License)
		}
		if len(meta) > 0 {
			fmt.Printf("%s\n\n", strings.Join(meta, " · "))
		}
	}
	return nil
}

// --- json output ---

func outputJSON(results []searchResult, total int) error {
	out := struct {
		Total   int         `json:"total"`
		Results []IdeaEntry `json:"results"`
	}{Total: total}
	for _, r := range results {
		out.Results = append(out.Results, r.entry)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// --- image saving ---

func saveIdeaImages(entries []IdeaEntry) ([]string, error) {
	var saved []string
	if err := os.MkdirAll(shared.OutputDir, 0755); err != nil {
		return saved, fmt.Errorf("cannot create output directory: %w", err)
	}
	for _, e := range entries {
		for _, imgURL := range e.ImageURLs {
			if imgURL == "" {
				continue
			}
			name := filepath.Base(imgURL)
			path := filepath.Join(shared.OutputDir, name)
			// Skip if already exists
			if _, err := os.Stat(path); err == nil {
				saved = append(saved, path)
				continue
			}
			data, err := downloadImage(imgURL)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to download %s: %v\n", imgURL, err)
				continue
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to save %s: %v\n", name, err)
				continue
			}
			saved = append(saved, path)
		}
	}
	return saved, nil
}

func localImagePath(remoteURL string) string {
	if remoteURL == "" {
		return ""
	}
	return filepath.Join(shared.OutputDir, filepath.Base(remoteURL))
}

// downloadImage downloads a URL to a byte slice with browser-like headers
// and retry on transient errors (EOF, connection reset).
// Inherits proxy settings from http.DefaultClient (configured by ConfigureDefaultClient).
func downloadImage(url string) ([]byte, error) {
	// Use DefaultClient's transport to inherit proxy configuration;
	// fall back to http.DefaultTransport if DefaultClient was not customized.
	transport := http.DefaultClient.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	var lastErr error
	for attempt := range 3 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		// Browser-like User-Agent to avoid CDN blocking
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
		// Set Referer for Twitter CDN images
		if strings.Contains(url, "twimg.com") || strings.Contains(url, "x.com") {
			req.Header.Set("Referer", "https://x.com/")
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			// Retry on EOF or connection reset — transient CDN issues
			if strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "connection reset") {
				continue
			}
			return nil, err
		}

		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf("failed after 3 attempts: %w", lastErr)
}

func init() {
	f := ideasCmd.Flags()
	f.IntVarP(&ideasLimit, "limit", "l", ideasDefaultLimit, "Number of results to show (default 8)")
	f.BoolVar(&ideasRandom, "random", false, "Shuffle matched results randomly (default: ranked by relevance)")
	f.BoolVar(&ideasJSON, "json", false, "Output as JSON instead of markdown")
	f.BoolVar(&ideasSaveImages, "save", false, "Download reference images to local directory")
	f.BoolVar(&ideasPreview, "preview", false, "Open saved images with system default viewer (implies --save)")
	f.StringVar(&ideasFindImage, "find-image", "", "Search by image filename (matches image_urls in dataset)")

	rootCmd.AddCommand(ideasCmd)
}
