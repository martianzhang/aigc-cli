package vision

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Tokenizer implements GPT-2's byte-level BPE tokenizer, used by Florence-2.
//
// Usage:
//
//	tk, err := NewTokenizer(vocabPath, mergesPath)
//	ids := tk.Encode("<DETAILED_CAPTION>")
//	text := tk.Decode(ids)
type Tokenizer struct {
	encoder     map[string]int    // token → id
	decoder     map[int]string    // id → token
	bpeRanks    map[string]int    // BPE merge pair → rank
	byteEncoder map[byte]string   // byte → unicode char
	byteDecoder map[string]byte   // unicode char → byte
	pattern     *regexp.Regexp    // pre-tokenization regex
	cache       map[string]string // BPE cache
}

// NewTokenizer loads a GPT-2 BPE tokenizer from vocab.json and merges.txt.
// vocabPath is the path to vocab.json, mergesPath is the path to merges.txt.
func NewTokenizer(vocabPath, mergesPath string) (*Tokenizer, error) {
	tk := &Tokenizer{
		encoder:  make(map[string]int),
		decoder:  make(map[int]string),
		bpeRanks: make(map[string]int),
		cache:    make(map[string]string),
	}

	if err := tk.loadVocab(vocabPath); err != nil {
		return nil, fmt.Errorf("load vocab: %w", err)
	}
	if err := tk.loadMerges(mergesPath); err != nil {
		return nil, fmt.Errorf("load merges: %w", err)
	}

	tk.buildByteEncoder()
	tk.pattern = regexp.MustCompile(`'s|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+|\s+`)
	return tk, nil
}

// loadVocab loads vocab.json (token → id mapping).
func (tk *Tokenizer) loadVocab(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &tk.encoder); err != nil {
		return err
	}
	for k, v := range tk.encoder {
		tk.decoder[v] = k
	}
	return nil
}

// loadMerges loads merges.txt (BPE merge pairs, one per line).
func (tk *Tokenizer) loadMerges(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#version") {
			continue
		}
		parts := strings.Split(line, " ")
		if len(parts) != 2 {
			continue
		}
		tk.bpeRanks[parts[0]+" "+parts[1]] = i
	}
	return nil
}

// bytesToUnicode builds the byte-to-unicode mapping used by GPT-2.
// It maps bytes 0-255 to printable unicode codepoints.
func bytesToUnicode() map[byte]string {
	m := make(map[byte]string)
	n := 0
	for b := 0; b < 256; b++ {
		if b < 33 || b == 127 || (b > 160 && b < 173) || (b > 173 && b < 256) {
			// Control characters and DEL: map to unicode starting from 256
			m[byte(b)] = string(rune(256 + n))
			n++
		} else {
			m[byte(b)] = string(rune(b))
		}
	}
	return m
}

func (tk *Tokenizer) buildByteEncoder() {
	tk.byteEncoder = bytesToUnicode()
	tk.byteDecoder = make(map[string]byte)
	for k, v := range tk.byteEncoder {
		tk.byteDecoder[v] = k
	}
}

// Encode converts a text string to a slice of token IDs with BOS/EOS.
func (tk *Tokenizer) Encode(text string) []int64 {
	// First pass: try to match the entire text as a single vocab entry.
	if id, ok := tk.encoder[text]; ok {
		return []int64{0, int64(id), 2} // BOS, token, EOS
	}

	// Pre-tokenize using the GPT-2 regex pattern
	words := tk.pattern.FindAllString(text, -1)
	if words == nil {
		words = []string{text}
	}

	var ids []int64
	// Add BOS
	ids = append(ids, 0)

	for _, word := range words {
		if id, ok := tk.encoder[word]; ok {
			ids = append(ids, int64(id))
			continue
		}

		var b strings.Builder
		for i := 0; i < len(word); i++ {
			b.WriteString(tk.byteEncoder[word[i]])
		}
		bpeStr := b.String()

		if id, ok := tk.encoder[bpeStr]; ok {
			ids = append(ids, int64(id))
			continue
		}

		bpeTokens := tk.bpe(bpeStr)
		for _, token := range bpeTokens {
			if id, ok := tk.encoder[token]; ok {
				ids = append(ids, int64(id))
			}
		}
	}

	// Add EOS
	ids = append(ids, 2)
	return ids
}

// Decode converts a slice of token IDs back to text.
func (tk *Tokenizer) Decode(ids []int64) string {
	var b strings.Builder
	for _, id := range ids {
		if token, ok := tk.decoder[int(id)]; ok {
			for i := 0; i < len(token); i++ {
				r, size := utf8.DecodeRuneInString(token[i:])
				char := string(r)
				if byteChar, ok := tk.byteDecoder[char]; ok {
					b.WriteByte(byteChar)
				} else {
					b.WriteString(char)
				}
				i += size - 1
			}
		}
	}
	return b.String()
}

// bpe applies BPE merge operations to a word, returning subword tokens.
func (tk *Tokenizer) bpe(word string) []string {
	if cached, ok := tk.cache[word]; ok {
		return strings.Split(cached, " ")
	}

	if len(word) <= 1 {
		tk.cache[word] = word
		return []string{word}
	}

	// Split into character pairs (as unicode characters)
	var chars []string
	for i := 0; i < len(word); {
		r, size := utf8.DecodeRuneInString(word[i:])
		chars = append(chars, string(r))
		i += size
	}

	if len(chars) <= 1 {
		tk.cache[word] = word
		return []string{word}
	}

	// Find the best merge pair (lowest rank)
	for len(chars) > 1 {

		bestPair := ""
		bestRank := int(^uint(0) >> 1) // max int

		for i := 0; i < len(chars)-1; i++ {
			pair := chars[i] + " " + chars[i+1]
			if rank, ok := tk.bpeRanks[pair]; ok && rank < bestRank {
				bestPair = pair
				bestRank = rank
			}
		}

		if bestPair == "" {
			break // no more merges
		}

		// Merge the best pair
		parts := strings.Split(bestPair, " ")
		var newChars []string
		for i := 0; i < len(chars); i++ {
			if i < len(chars)-1 && chars[i] == parts[0] && chars[i+1] == parts[1] {
				newChars = append(newChars, parts[0]+parts[1])
				i++ // skip the next char since it was merged
			} else {
				newChars = append(newChars, chars[i])
			}
		}
		chars = newChars
	}

	result := strings.Join(chars, " ")
	tk.cache[word] = result

	// Split back, but ensure we don't lose valid single-character tokens
	if len(chars) == 0 {
		return []string{word}
	}
	return chars
}

// VocabSize returns the vocabulary size.
func (tk *Tokenizer) VocabSize() int {
	return len(tk.encoder)
}

// EncodeTokens converts a string to token strings (for debugging).
func (tk *Tokenizer) EncodeTokens(text string) []string {
	words := tk.pattern.FindAllString(text, -1)
	if words == nil {
		words = []string{text}
	}

	var tokens []string
	for _, word := range words {
		var b strings.Builder
		for i := 0; i < len(word); i++ {
			b.WriteString(tk.byteEncoder[word[i]])
		}
		bpeTokens := tk.bpe(b.String())
		tokens = append(tokens, bpeTokens...)
	}
	return tokens
}
