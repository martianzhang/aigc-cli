package knowledge

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"unicode"
)

type Tokenizer struct {
	vocab  map[string]int
	scores map[string]float64
	unkID  int
	bosID  int
	eosID  int
	padID  int
	maxLen int
}

type tokJSON struct {
	Version string `json:"version"`
	Model   struct {
		Type   string          `json:"type"`
		Vocab  json.RawMessage `json:"vocab"` // can be object or array
		Scores []float64       `json:"scores"`
	} `json:"model"`
	AddedTokens []struct {
		ID         int    `json:"id"`
		Content    string `json:"content"`
		SingleWord bool   `json:"single_word"`
	} `json:"added_tokens"`
}

func NewTokenizer(data []byte) (*Tokenizer, error) {
	var t tokJSON
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse tokenizer.json: %w", err)
	}

	tk := &Tokenizer{
		vocab:  make(map[string]int),
		scores: make(map[string]float64),
		unkID:  0,
		bosID:  0,
		eosID:  2,
		padID:  1,
		maxLen: 512,
	}

	// Vocab can be an object {token: id} or an array [[token, score], ...]
	if err := tk.loadVocab(t.Model.Vocab, t.Model.Scores); err != nil {
		return nil, err
	}

	for _, at := range t.AddedTokens {
		tk.vocab[at.Content] = at.ID
	}

	if id, ok := tk.vocab["<unk>"]; ok {
		tk.unkID = id
	}
	if id, ok := tk.vocab["<s>"]; ok {
		tk.bosID = id
	}
	if id, ok := tk.vocab["</s>"]; ok {
		tk.eosID = id
	}
	if id, ok := tk.vocab["<pad>"]; ok {
		tk.padID = id
	}

	return tk, nil
}

func (t *Tokenizer) loadVocab(raw json.RawMessage, scores []float64) error {
	// Try object format first {token: id}
	var obj map[string]int
	if err := json.Unmarshal(raw, &obj); err == nil {
		for token, id := range obj {
			t.vocab[token] = id
			if id >= 0 && id < len(scores) {
				t.scores[token] = scores[id]
			}
		}
		return nil
	}

	// Try array format [[token, score], ...]
	var arr []interface{}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return fmt.Errorf("vocab: expected object or array")
	}
	for i, entry := range arr {
		pair, ok := entry.([]interface{})
		if !ok || len(pair) < 1 {
			continue
		}
		token, ok := pair[0].(string)
		if !ok {
			continue
		}
		t.vocab[token] = i
		if len(pair) > 1 {
			if score, ok := pair[1].(float64); ok {
				t.scores[token] = score
			}
		}
	}
	return nil
}

func (t *Tokenizer) Encode(text string) []int64 {
	ids := t.encodeText(text)
	maxIDs := t.maxLen - 2
	if len(ids) > maxIDs {
		ids = ids[:maxIDs]
	}
	result := make([]int64, 0, len(ids)+2)
	result = append(result, int64(t.bosID))
	for _, id := range ids {
		result = append(result, int64(id))
	}
	result = append(result, int64(t.eosID))
	return result
}

func (t *Tokenizer) encodeText(text string) []int {
	words := preTokenizeUnigram(text)
	var ids []int
	for _, word := range words {
		tokens := t.unigramTokenize(word)
		for _, tok := range tokens {
			if id, ok := t.vocab[tok]; ok {
				ids = append(ids, id)
			} else {
				ids = append(ids, t.unkID)
			}
		}
	}
	return ids
}

func (t *Tokenizer) unigramTokenize(word string) []string {
	if word == "" {
		return nil
	}
	if _, ok := t.vocab[word]; ok {
		return []string{word}
	}

	runes := []rune(word)
	n := len(runes)

	bestScore := make([]float64, n+1)
	bestSplit := make([]int, n+1)
	for i := range bestScore {
		bestScore[i] = math.Inf(-1)
	}
	bestScore[0] = 0

	for end := 1; end <= n; end++ {
		for start := 0; start < end; start++ {
			piece := string(runes[start:end])
			score, inVocab := t.scores[piece]
			if !inVocab {
				continue
			}
			if math.IsInf(bestScore[start], -1) {
				continue
			}
			candidate := bestScore[start] + score
			if candidate > bestScore[end] {
				bestScore[end] = candidate
				bestSplit[end] = start
			}
		}
	}

	var tokens []string
	end := n
	for end > 0 {
		start := bestSplit[end]
		if start == 0 && end == n && math.IsInf(bestScore[end], -1) {
			break // no valid segmentation found
		}
		tokens = append([]string{string(runes[start:end])}, tokens...)
		end = start
	}

	if len(tokens) == 0 {
		return []string{word} // will become <unk>
	}
	return tokens
}

func preTokenizeUnigram(text string) []string {
	var words []string
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			words = append(words, buf.String())
			buf.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsSpace(r) {
			flush()
		} else if unicode.IsPunct(r) || unicode.IsSymbol(r) {
			flush()
			words = append(words, string(r))
		} else {
			buf.WriteRune(r)
		}
	}
	flush()
	return words
}

func (t *Tokenizer) Decode(ids []int64) string {
	rev := make(map[int]string)
	for token, id := range t.vocab {
		rev[id] = token
	}
	var parts []string
	for _, id := range ids {
		token, ok := rev[int(id)]
		if !ok {
			continue
		}
		if id == int64(t.bosID) || id == int64(t.eosID) || id == int64(t.padID) {
			continue
		}
		token = strings.ReplaceAll(token, "▁", " ")
		token = strings.TrimSpace(token)
		parts = append(parts, token)
	}
	return strings.Join(parts, "")
}

func (t *Tokenizer) VocabSize() int { return len(t.vocab) }
