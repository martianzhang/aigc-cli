package redact

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// ChatClient is the subset of client.APIClient used by the chatner engine.
type ChatClient interface {
	ChatCompletion(req *types.ChatRequest) (*types.ChatResponse, error)
}

const defaultChatnerPrompt = `Recognize the following entity types in the text.
Specified types:%s
<text>%s</text>`

// chatnerOccurrence controls how a model-returned value is located in text.
const (
	OccurrenceAnchored = "anchored"
	OccurrenceAll      = "all"
	OccurrenceFirst    = "first"
)

var anchorWords = []string{
	"原告", "被告", "申请人", "被申请人", "委托人", "受托人", "联系人",
	"甲方", "乙方", "丙方", "买方", "卖方", "客户", "员工",
	"姓名", "法定代表人", "负责人", "代理人", "律师", "证人", "被害人",
	"住址", "地址", "住所地", "注册地址", "电话", "手机", "邮箱",
}

// Chatner is a model-agnostic, value-level NER engine. It works with Ollama,
// llama-server, vLLM or any OpenAI-compatible endpoint.
type Chatner struct {
	client       ChatClient
	model        string
	types        []string
	prompt       string
	occurrence   string
	anchorWindow int
}

// NewChatner builds a chatner engine from config.
func NewChatner(c ChatClient, model string, cfg *types.RedactChatnerConfig) (*Chatner, error) {
	if c == nil {
		return nil, fmt.Errorf("chatner: client is required")
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("chatner: model is required")
	}
	e := &Chatner{
		client: c, model: model,
		occurrence: OccurrenceAnchored, anchorWindow: 20,
		types: []string{
			"人名", "组织", "地址", "电话", "身份证号", "银行卡号", "邮箱",
			"统一社会信用代码", "案号", "车牌号", "护照号", "日期", "金额", "律师执业证号",
		},
	}
	if cfg != nil {
		if len(cfg.Types) > 0 {
			e.types = cfg.Types
		}
		if cfg.Prompt != "" {
			e.prompt = cfg.Prompt
		}
		if cfg.Occurrence != "" {
			switch cfg.Occurrence {
			case OccurrenceAnchored, OccurrenceAll, OccurrenceFirst:
				e.occurrence = cfg.Occurrence
			default:
				return nil, fmt.Errorf("chatner.occurrence: unknown value %q", cfg.Occurrence)
			}
		}
		if cfg.AnchorWindow > 0 {
			e.anchorWindow = cfg.AnchorWindow
		}
	}
	return e, nil
}

func (e *Chatner) Name() string { return "chatner" }

const (
	chatnerChunkRunes   = 3000
	chatnerChunkOverlap = 200
)

func (e *Chatner) Detect(ctx context.Context, doc *Document) (Result, error) {
	runes := []rune(doc.Text)
	var result Result
	for _, chunk := range chunkText(doc.Text, chatnerChunkRunes, chatnerChunkOverlap) {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		prompt := e.promptFor(chunk.Text)
		temp := 0.0
		req := &types.ChatRequest{
			Model:       e.model,
			Messages:    []types.ChatMessage{{Role: "user", Content: prompt}},
			Temperature: &temp,
		}
		resp, err := e.client.ChatCompletion(req)
		if err != nil {
			return Result{}, fmt.Errorf("chatner: %w", err)
		}
		if resp == nil || len(resp.Choices) == 0 {
			continue
		}
		parsed := parseEntityJSON(resp.Choices[0].Message.Content)
		for label, values := range parsed {
			typ := CanonicalType(label)
			for _, value := range values {
				value = strings.TrimSpace(value)
				if value == "" {
					continue
				}
				for _, pos := range locateValue(chunk.Text, value, e.occurrence) {
					start, end := pos[0]+chunk.Start, pos[1]+chunk.Start
					span := Span{
						Start: start, End: end, Type: typ, Text: value,
						Rule: "chatner", Source: SourceModel, Score: 0.9, Priority: 55,
					}
					if e.occurrence == OccurrenceAnchored && !anchored(runes, doc, span, e.anchorWindow) {
						result.Review = append(result.Review, span)
						continue
					}
					result.Spans = append(result.Spans, span)
				}
			}
		}
	}
	result.Spans = dedupeSpans(result.Spans)
	result.Review = dedupeSpans(result.Review)
	if e.occurrence == OccurrenceFirst {
		sortSpans(result.Spans)
		if len(result.Spans) > 1 {
			result.Spans = result.Spans[:1]
		}
	}
	return result, nil
}

func (e *Chatner) promptFor(text string) string {
	if e.prompt == "" {
		typesJSON, _ := json.Marshal(e.types)
		return fmt.Sprintf(defaultChatnerPrompt, string(typesJSON), text)
	}
	if strings.Contains(e.prompt, "{text}") {
		return strings.ReplaceAll(e.prompt, "{text}", text)
	}
	return e.prompt + "\n<text>" + text + "</text>"
}

type textChunk struct {
	Text  string
	Start int // byte offset in the original text
}

func chunkText(text string, maxRunes, overlap int) []textChunk {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return []textChunk{{Text: text, Start: 0}}
	}
	// Precompute byte offsets so chunking stays O(n) on large documents.
	starts := make([]int, len(runes)+1)
	off := 0
	for i, r := range runes {
		starts[i] = off
		off += utf8.RuneLen(r)
	}
	starts[len(runes)] = off

	var out []textChunk
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			// Prefer a sentence boundary in the second half of the window.
			minEnd := start + maxRunes/2
			for i := end; i > minEnd; i-- {
				if isSentenceEnd(runes[i-1]) {
					end = i
					break
				}
			}
		}
		out = append(out, textChunk{
			Text:  text[starts[start]:starts[end]],
			Start: starts[start],
		})
		if end >= len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = start + 1
		}
		start = next
	}
	return out
}

func sortSpans(spans []Span) {
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
}

func locateValue(text, value, mode string) [][2]int {
	var out [][2]int
	start := 0
	for {
		i := strings.Index(text[start:], value)
		if i < 0 {
			break
		}
		s := start + i
		out = append(out, [2]int{s, s + len(value)})
		if mode == OccurrenceFirst {
			break
		}
		start = s + maxInt(1, len(value))
	}
	return out
}

func anchored(runes []rune, doc *Document, span Span, window int) bool {
	start := doc.RuneOffset(span.Start)
	end := doc.RuneOffset(span.End)
	lo, hi := start-window, end+window
	if lo < 0 {
		lo = 0
	}
	if hi > len(runes) {
		hi = len(runes)
	}
	// Do not let an anchor leak across a sentence boundary: "联系人：高峰。
	// 高峰时段…" must not anchor the second 高峰.
	for i := start - 1; i >= lo; i-- {
		if isSentenceEnd(runes[i]) {
			lo = i + 1
			break
		}
	}
	for i := end; i < hi; i++ {
		if isSentenceEnd(runes[i]) {
			hi = i
			break
		}
	}
	ctx := string(runes[lo:hi])
	for _, w := range anchorWords {
		if strings.Contains(ctx, w) {
			return true
		}
	}
	return false
}

func parseEntityJSON(content string) map[string][]string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return nil
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(content[start:end+1]), &raw); err != nil {
		return nil
	}
	out := map[string][]string{}
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			out[k] = []string{val}
		case []any:
			for _, item := range val {
				if s, ok := item.(string); ok {
					out[k] = append(out[k], s)
				}
			}
		}
	}
	return out
}

func isSentenceEnd(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '!', '?', ';', '\n':
		return true
	default:
		return false
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
