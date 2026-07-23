package ocr

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/sajari/fuzzy"
)

// defaultDictPath returns the platform's default system dictionary path.
func defaultDictPath() string {
	// macOS
	if _, err := os.Stat("/usr/share/dict/words"); err == nil {
		return "/usr/share/dict/words"
	}
	// Linux (aspell / debian)
	for _, p := range []string{
		"/usr/share/dict/american-english",
		"/usr/share/dict/english",
		"/usr/share/dict/words",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// LoadDictionary loads English words from the given file (one word per line)
// and trains the spellcheck model. If path is empty, tries the system dict.
// Returns the number of words loaded, or an error if the file cannot be read.
func (e *Engine) LoadDictionary(path string) (int, error) {
	if path == "" {
		path = defaultDictPath()
	}
	if path == "" {
		e.wsDictPath = ""
		e.wsWordSet = make(map[string]bool)
		return 0, fmt.Errorf("no dictionary file found; run 'aigc-cli ocr init' or set dict.path")
	}

	f, err := os.Open(path)
	if err != nil {
		e.wsDictPath = ""
		e.wsWordSet = make(map[string]bool)
		return 0, fmt.Errorf("open dictionary %s: %w", path, err)
	}
	defer f.Close()

	e.wsWordSet = make(map[string]bool)
	// Short common words (manually curated to avoid over-splitting by DP).
	for _, w := range strings.Fields(shortWords) {
		e.wsWordSet[w] = true
	}

	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		w := strings.TrimSpace(scanner.Text())
		// Only accept purely alphabetic words, 3-20 chars (short words cause DP over-split).
		if len(w) >= 3 && len(w) <= 20 {
			valid := true
			for _, r := range w {
				if r < 'a' || r > 'z' {
					valid = false
					break
				}
			}
			if valid {
				e.wsWordSet[w] = true
				count++
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return count, fmt.Errorf("read dictionary: %w", err)
	}

	e.wsDictPath = path

	return count, nil
}

// DictPath returns the loaded dictionary path, or empty string if none loaded.
func (e *Engine) DictPath() string { return e.wsDictPath }

// TryLoadDictionary loads the best available dictionary.
// Priority: downloaded dict_en_words.txt → system dictionary.
func (e *Engine) tryLoadDictionary() {
	if e.wsWordSet != nil {
		return
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, ".config", "aigc-cli", "models", "ocr", "dict_en_words.txt"),
		filepath.Join(home, ".config", "aigc-cli", "models", "dict_en_words.txt"),
	} {
		if _, err := os.Stat(p); err == nil {
			e.LoadDictionary(p)
			return
		}
	}
	if p := defaultDictPath(); p != "" {
		e.LoadDictionary(p)
	}
}

// splitEnglishWords splits concatenated English words in text using DP.
// Preserves paragraph structure (\n separators).
func (e *Engine) splitEnglishWords(text string) string {
	if e.wsWordSet == nil {
		e.tryLoadDictionary()
	}
	if len(e.wsWordSet) < 100 {
		return text
	}

	// Split by newlines to preserve paragraph structure.
	lines := strings.Split(text, "\n")
	changed := false
	for li, line := range lines {
		// Process each space-delimited token in the line independently.
		tokens := strings.Fields(line)
		for i, tok := range tokens {
			if len(tok) < 10 {
				continue
			}
			ascii := 0
			for _, r := range tok {
				if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
					ascii++
				}
			}
			if float64(ascii)/float64(len(tok)) < 0.7 {
				continue
			}
			clean := make([]byte, 0, len(tok))
			for _, r := range tok {
				if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
					clean = append(clean, byte(r))
				}
			}
			if len(clean) < 10 {
				continue
			}
			split := e.dpWordSplit(string(clean))
			if split != string(clean) {
				tokens[i] = split
				changed = true
			}
		}
		lines[li] = strings.Join(tokens, " ")
	}
	if !changed {
		return text
	}
	return strings.Join(lines, "\n")
}

// SpellcheckText applies automatic spelling correction to OCR output.
// Preserves paragraph structure (\n separators).
func (e *Engine) SpellcheckText(text string) string {
	if e.wsWordSet == nil {
		e.tryLoadDictionary()
	}
	if len(e.wsWordSet) < 100 {
		return text
	}
	lines := strings.Split(text, "\n")
	changed := false
	for li, line := range lines {
		words := strings.Fields(line)
		for i, w := range words {
			corrected := e.spellcheckWord(w)
			if corrected != w {
				words[i] = corrected
				changed = true
			}
		}
		lines[li] = strings.Join(words, " ")
	}
	if !changed {
		return text
	}
	return strings.Join(lines, "\n")
}

// ensureSpellcheckModel lazily initializes the fuzzy model on first use.
func (e *Engine) ensureSpellcheckModel() {
	if e.wsSpellcheckModel != nil || len(e.wsWordSet) < 100 {
		return
	}
	m := fuzzy.NewModel()
	m.SetThreshold(1)
	m.SetDepth(1)
	words := make([]string, 0, len(e.wsWordSet))
	for w := range e.wsWordSet {
		words = append(words, w)
	}
	m.Train(words)
	e.wsSpellcheckModel = m
}

func (e *Engine) spellcheckWord(word string) string {
	if word == "" {
		return word
	}
	clean, suffix := stripPunctuation(word)
	if clean == "" {
		return word
	}
	if e.isKnownWord(clean) || isAllDigitsOrPunct(clean) {
		return word
	}
	lower := strings.ToLower(clean)
	if e.isKnownWord(lower) {
		return word
	}
	// Skip proper nouns: mixed-case words that are already known words
	// when lowercased are likely intentional. E.g. "Albase" (brand name)
	// shouldn't be spellchecked to "abase".
	if clean != lower && len(clean) >= 4 {
		return word
	}

	ocrFixed := fixOCRTypo(lower)
	if ocrFixed != lower {
		if e.wsWordSet[ocrFixed] || e.isKnownWord(ocrFixed) {
			return restoreCase(ocrFixed, clean, suffix)
		}
	}

	// Conservative spellcheck: only correct short-range errors.
	// Skip very short words and words likely to be abbreviations.
	// Try character-level OCR disambiguation on the original-case word.
	// Pass `lower` as fallback dict key so dictionary lookups work.
	if swapped := e.fixCharConfusion(clean, lower); swapped != clean {
		return restoreCase(swapped, clean, suffix)
	}

	if len(lower) >= 4 {
		e.ensureSpellcheckModel()
	}
	if e.wsSpellcheckModel != nil && len(lower) >= 4 {
		suggestions := e.wsSpellcheckModel.SpellCheckSuggestions(lower, 3)
		best := ""
		for _, s := range suggestions {
			if s == "" || s == lower {
				continue
			}
			d := levenshtein(lower, s)
			if d <= 1 {
				best = s
				break
			}
		}
		if best != "" {
			return restoreCase(best, clean, suffix)
		}
	}
	return word
}

func stripPunctuation(word string) (clean, suffix string) {
	for len(word) > 0 {
		r := rune(word[len(word)-1])
		if r == '.' || r == ',' || r == '!' || r == '?' || r == ';' || r == ':' ||
			r == ')' || r == ']' || r == '"' || r == '\'' || r == '\u201D' || r == '\u2019' {
			suffix = string(r) + suffix
			word = word[:len(word)-1]
		} else {
			break
		}
	}
	return word, suffix
}

func restoreCase(corrected, original, suffix string) string {
	if original == strings.ToLower(original) {
		return corrected + suffix
	}
	// Short words corrected via confusion sets are likely acronyms: "Al" → "AI".
	if len(original) <= 3 {
		return strings.ToUpper(corrected) + suffix
	}
	r := []rune(corrected)
	if len(r) > 0 && unicode.IsUpper(rune(original[0])) {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r) + suffix
}

// dpWordSplit uses dynamic programming to find the optimal word segmentation.
func (e *Engine) dpWordSplit(text string) string {
	n := len(text)
	if n == 0 {
		return text
	}
	const maxWordLen = 30
	penaltyPerChar := -6.0
	const wordPenalty = -15.0

	dp := make([]float64, n+1)
	prev := make([]int, n+1)
	dp[0] = 0
	prev[0] = -1
	for i := 1; i <= n; i++ {
		dp[i] = float64(-i) * 20
		prev[i] = -1
	}

	for i := 1; i <= n; i++ {
		start := i - maxWordLen
		if start < 0 {
			start = 0
		}
		for j := start; j < i; j++ {
			word := text[j:i]
			score := dp[j] + wordPenalty
			lower := strings.ToLower(word)

			if e.isKnownWord(word) || (len(word) >= 3 && e.wsWordSet[lower]) {
				score += float64(len(word)) * float64(len(word))
			} else if isAllDigitsOrPunct(word) {
				score += float64(len(word)) * float64(len(word)) * 0.5
			} else {
				score += float64(len(word)) * penaltyPerChar
			}

			if score > dp[i] {
				dp[i] = score
				prev[i] = j
			}
		}
	}

	var words []string
	i := n
	for i > 0 {
		j := prev[i]
		if j < 0 {
			j = i - 1
		}
		words = append(words, text[j:i])
		i = j
	}
	for left, right := 0, len(words)-1; left < right; left, right = left+1, right-1 {
		words[left], words[right] = words[right], words[left]
	}
	// For short tokens (≤ 14 chars after cleaning), reject the split unless
	// ALL resulting parts are known dictionary words. This prevents splitting
	// product names (e.g., "CodeBuddyNPC") while allowing genuine merges
	// in longer text.
	if len(words) > 1 && len(text) <= 14 {
		allKnown := true
		for _, w := range words {
			lower := strings.ToLower(w)
			if e.isKnownWord(w) || isAllDigitsOrPunct(w) {
				continue
			}
			if len(lower) >= 2 && e.wsWordSet[lower] {
				continue
			}
			allKnown = false
			break
		}
		if !allKnown {
			return text
		}
	}
	return strings.Join(words, " ")
}

// isKnownWord checks if a word is in the English dictionary.
func (e *Engine) isKnownWord(word string) bool {
	if word == "" {
		return false
	}
	lower := strings.ToLower(word)
	if len(lower) == 1 && lower != "a" && lower != "i" {
		return false
	}
	if e.wsWordSet[lower] {
		return true
	}
	if strings.HasSuffix(lower, "'s") {
		if e.wsWordSet[lower[:len(lower)-2]] {
			return true
		}
	}
	if strings.HasSuffix(lower, "s") && len(lower) > 3 {
		// Plural: word + s
		if e.wsWordSet[lower[:len(lower)-1]] {
			return true
		}
		// -ies → -y: companies → company
		if strings.HasSuffix(lower, "ies") && len(lower) > 4 {
			if e.wsWordSet[lower[:len(lower)-3]+"y"] {
				return true
			}
		}
	}
	if strings.HasSuffix(lower, "ed") && len(lower) > 4 {
		root := lower[:len(lower)-2]
		if e.wsWordSet[root] {
			return true
		}
		if e.wsWordSet[lower[:len(lower)-1]] {
			return true
		}
	}
	if strings.HasSuffix(lower, "ing") && len(lower) > 5 {
		root := lower[:len(lower)-3]
		if e.wsWordSet[root] {
			return true
		}
		if e.wsWordSet[root+"e"] {
			return true
		}
	}
	if strings.HasSuffix(lower, "er") && len(lower) > 4 {
		if e.wsWordSet[lower[:len(lower)-2]] {
			return true
		}
	}
	if strings.HasSuffix(lower, "est") && len(lower) > 5 {
		if e.wsWordSet[lower[:len(lower)-3]] {
			return true
		}
	}
	return false
}

// levenshtein computes the Levenshtein edit distance.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	if la > lb {
		a, b = b, a
		la, lb = lb, la
	}
	prev := make([]int, la+1)
	curr := make([]int, la+1)
	for i := range prev {
		prev[i] = i
	}
	for j := 1; j <= lb; j++ {
		curr[0] = j
		for i := 1; i <= la; i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[i] = min3(prev[i]+1, curr[i-1]+1, prev[i-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[la]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func isAllDigitsOrPunct(word string) bool {
	if word == "" {
		return false
	}
	for _, r := range word {
		if !unicode.IsDigit(r) && r != '.' && r != ',' && r != '%' && r != '$' && r != '€' && r != '£' && r != '¥' {
			return false
		}
	}
	return true
}

func fixOCRTypo(word string) string {
	if len(word) < 2 {
		return word
	}
	subs := []struct {
		old, new string
	}{
		{"rn", "m"},
		{"cl", "d"},
		{"1", "l"},
		{"0", "o"},
		{"5", "s"},
		{"8", "b"},
		{"vv", "w"},
	}
	result := word
	for _, s := range subs {
		candidate := strings.ReplaceAll(result, s.old, s.new)
		if candidate != result {
			if len(candidate) <= len(result) || countVowels(candidate) >= countVowels(result) {
				result = candidate
			}
		}
	}
	return result
}

// charConfusionSets groups characters that OCR models commonly confuse
// because they look nearly identical in many fonts (i/l/I/1, 0/O, etc.).
// Within each set, alternatives with the same case category are tried first.
var charConfusionSets = []string{
	"iIl1", // lowercase i, uppercase I, lowercase L, digit one
	"0Oo",  // digit zero, uppercase O, lowercase o
}

// fixCharConfusion tries replacing each character with alternatives from
// the same confusion set. `word` is the original-case word; `lowerKey` is
// the lowercase version used for dictionary lookups.
func (e *Engine) fixCharConfusion(word, lowerKey string) string {
	if len(word) < 2 {
		return word
	}
	runes := []rune(word)
	for pos, r := range runes {
		for _, set := range charConfusionSets {
			idx := -1
			for si, c := range set {
				if c == r {
					idx = si
					break
				}
			}
			if idx < 0 {
				continue
			}
			// Try each alternative, preferring same-case replacements.
			for _, alt := range orderByCase(r, set, idx) {
				runes[pos] = alt
				candidate := string(runes)
				if e.wsWordSet[strings.ToLower(candidate)] || e.isKnownWord(candidate) {
					return candidate
				}
				runes[pos] = r // restore
			}
		}
	}
	return word
}

// orderByCase returns confusion-set alternatives ordered so that
// same-case alternatives come first.
func orderByCase(original rune, set string, skipIdx int) []rune {
	isLower := original >= 'a' && original <= 'z'
	isUpper := original >= 'A' && original <= 'Z'
	// Two passes: same-category first, then others.
	var result, others []rune
	for si, c := range set {
		if si == skipIdx {
			continue
		}
		if (isLower && c >= 'a' && c <= 'z') || (isUpper && c >= 'A' && c <= 'Z') {
			result = append(result, c)
		} else {
			others = append(others, c)
		}
	}
	result = append(result, others...)
	if result == nil {
		return []rune{}
	}
	return result
}

func countVowels(s string) int {
	count := 0
	for _, r := range s {
		switch r {
		case 'a', 'e', 'i', 'o', 'u', 'y':
			count++
		}
	}
	return count
}

// shortWords is the curated list of common 1-2 letter English words for the
// dictionary. Longer words (3-20 chars) come from the system dictionary file.
const shortWords = "a i ai am an as at be by do go he if in is it me my no of on or so to up us we has had not but all can any are and for the was got say get new now out did get may its see too way who old put set let run yet man men far got jan feb mar apr jun jul aug sep oct nov dec"
