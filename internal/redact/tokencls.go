package redact

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ort "github.com/amikos-tech/pure-onnx/ort"
	tk "github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/types"
)

const (
	defaultTokenclsMaxLen = 512
	defaultTokenclsStride = 128
)

// TokenClassifier runs a local ONNX token-classification model (BIO/BIOES).
// Model directory layout:
//
//	model.onnx  config.json (id2label)  tokenizer.json
type TokenClassifier struct {
	tokenizer  *tk.Tokenizer
	session    *ort.AdvancedSession
	inputIDs   *ort.Tensor[int64]
	attention  *ort.Tensor[int64]
	typeIDs    *ort.Tensor[int64]
	output     *ort.Tensor[float32]
	labels     []string
	maxLen     int
	stride     int
	biases     tokenTransitionBiases
	hasTypeIDs bool
	clsID      int
	sepID      int
	padID      int
}

type tokenItem struct {
	startChar int
	endChar   int
	id        int
	typeID    int
}

// NewTokenClassifier loads a model from cfg.ModelDir or modelsDir/redact/<model>.
func NewTokenClassifier(cfg *types.RedactTokenclsConfig, modelsDir string) (*TokenClassifier, error) {
	if cfg == nil {
		cfg = &types.RedactTokenclsConfig{}
	}
	dir := cfg.ModelDir
	if dir == "" {
		name := cfg.Model
		if name == "" {
			name = "default"
		}
		dir = filepath.Join(modelsDir, "redact", name)
	}
	modelPath := filepath.Join(dir, "model.onnx")
	if _, err := os.Stat(modelPath); err != nil { // #nosec G703 -- model path comes from local config
		return nil, fmt.Errorf("tokencls model not found at %s: %w", modelPath, err)
	}
	cfgPath := filepath.Join(dir, "config.json")
	tokPath := filepath.Join(dir, "tokenizer.json")
	cfgData, err := os.ReadFile(cfgPath) // #nosec G703 -- model path comes from local config
	if err != nil {
		return nil, fmt.Errorf("read tokencls config: %w", err)
	}
	var modelCfg struct {
		ID2Label map[string]string `json:"id2label"`
	}
	if err := json.Unmarshal(cfgData, &modelCfg); err != nil {
		return nil, fmt.Errorf("parse tokencls config: %w", err)
	}
	labels, err := orderedLabels(modelCfg.ID2Label)
	if err != nil {
		return nil, err
	}
	tokenizer, err := pretrained.FromFile(tokPath)
	if err != nil {
		return nil, fmt.Errorf("load tokencls tokenizer: %w", err)
	}

	maxLen := cfg.MaxLen
	if maxLen <= 2 {
		maxLen = defaultTokenclsMaxLen
	}
	stride := cfg.Stride
	if stride <= 0 {
		stride = defaultTokenclsStride
	}
	if stride >= maxLen {
		stride = maxLen / 2
	}
	if stride < 1 {
		stride = 1
	}

	libPath, err := onnxrt.LibPath(modelsDir)
	if err != nil {
		return nil, fmt.Errorf("ONNX Runtime not found (%v); run 'aigc-cli ocr init' first", err)
	}
	if err := onnxrt.InitEnvironment(libPath); err != nil {
		return nil, err
	}

	clsID, _ := tokenizer.TokenToId("[CLS]")
	if clsID == 0 {
		if id, ok := tokenizer.TokenToId("<s>"); ok {
			clsID = id
		}
	}
	sepID, _ := tokenizer.TokenToId("[SEP]")
	if sepID == 0 {
		if id, ok := tokenizer.TokenToId("</s>"); ok {
			sepID = id
		}
	}
	padID, _ := tokenizer.TokenToId("[PAD]")
	if padID == 0 {
		if id, ok := tokenizer.TokenToId("<pad>"); ok {
			padID = id
		}
	}

	inputSets := [][]string{cfg.Inputs}
	if len(cfg.Inputs) == 0 {
		inputSets = [][]string{
			{"input_ids", "attention_mask", "token_type_ids"},
			{"input_ids", "attention_mask"},
		}
	}
	outputs := []string{cfg.Output}
	if cfg.Output == "" {
		outputs = []string{"logits", "output", "emissions"}
	}

	biases, err := resolveTokenBiases(cfg)
	if err != nil {
		return nil, err
	}
	tc := &TokenClassifier{
		tokenizer: tokenizer, labels: labels, maxLen: maxLen, stride: stride,
		biases: biases, clsID: clsID, sepID: sepID, padID: padID,
	}
	var lastErr error
	for _, inputs := range inputSets {
		hasTypes := len(inputs) >= 3
		ids, attn, types, out, err := tc.allocate(hasTypes)
		if err != nil {
			return nil, err
		}
		success := false
		for _, outputName := range outputs {
			values := []ort.Value{ids, attn}
			if types != nil {
				values = append(values, types)
			}
			session, err := ort.NewAdvancedSession(modelPath, inputs,
				[]string{outputName}, values, []ort.Value{out}, nil)
			if err == nil {
				if probeErr := probeTokenclsSession(session, ids, attn, types); probeErr != nil {
					_ = session.Destroy()
					lastErr = probeErr
					continue
				}
				tc.session, tc.inputIDs, tc.attention, tc.typeIDs, tc.output = session, ids, attn, types, out
				tc.hasTypeIDs = hasTypes
				success = true
				break
			}
			lastErr = err
		}
		if success {
			return tc, nil
		}
		_ = ids.Destroy()
		_ = attn.Destroy()
		if types != nil {
			_ = types.Destroy()
		}
		_ = out.Destroy()
	}
	return nil, fmt.Errorf("create tokencls session: %w", lastErr)
}

// tokenEndRune normalizes the tokenizer end offset. sugarme's
// EncodeCharOffsets sometimes reports an inclusive end (e.g. a multi-character
// token) and sometimes an exclusive end (e.g. a single CJK character). Compare
// the token text with both candidates instead of assuming one convention.
func tokenEndRune(runes []rune, start, end int, token string) int {
	if end < start {
		return start
	}
	if end > len(runes) {
		end = len(runes)
	}
	if end == start || end == len(runes) {
		return end
	}
	needle := strings.TrimPrefix(token, "##")
	needle = strings.TrimPrefix(needle, "▁")
	if needle == "" || strings.HasPrefix(needle, "[") {
		return end
	}
	if strings.EqualFold(string(runes[start:end]), needle) {
		return end
	}
	if strings.EqualFold(string(runes[start:end+1]), needle) {
		return end + 1
	}
	return end
}

func probeTokenclsSession(session *ort.AdvancedSession, ids, attn, types *ort.Tensor[int64]) error {
	for _, v := range []*ort.Tensor[int64]{ids, attn, types} {
		if v == nil {
			continue
		}
		for i := range v.GetData() {
			v.GetData()[i] = 1
		}
	}
	if err := session.Run(); err != nil {
		return fmt.Errorf("probe tokencls session: %w", err)
	}
	return nil
}

func (tc *TokenClassifier) allocate(withTypes bool) (*ort.Tensor[int64], *ort.Tensor[int64], *ort.Tensor[int64], *ort.Tensor[float32], error) {
	shape := ort.NewShape(1, int64(tc.maxLen))
	ids, err := ort.NewTensor[int64](shape, make([]int64, tc.maxLen))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("create ids tensor: %w", err)
	}
	attn, err := ort.NewTensor[int64](shape, make([]int64, tc.maxLen))
	if err != nil {
		_ = ids.Destroy()
		return nil, nil, nil, nil, fmt.Errorf("create attention tensor: %w", err)
	}
	var typesTensor *ort.Tensor[int64]
	if withTypes {
		typesTensor, err = ort.NewTensor[int64](shape, make([]int64, tc.maxLen))
		if err != nil {
			_ = ids.Destroy()
			_ = attn.Destroy()
			return nil, nil, nil, nil, fmt.Errorf("create type_ids tensor: %w", err)
		}
	}
	outShape := ort.NewShape(1, int64(tc.maxLen), int64(len(tc.labels)))
	out, err := ort.NewTensor[float32](outShape, make([]float32, tc.maxLen*len(tc.labels)))
	if err != nil {
		_ = ids.Destroy()
		_ = attn.Destroy()
		if typesTensor != nil {
			_ = typesTensor.Destroy()
		}
		return nil, nil, nil, nil, fmt.Errorf("create logits tensor: %w", err)
	}
	return ids, attn, typesTensor, out, nil
}

func orderedLabels(id2label map[string]string) ([]string, error) {
	if len(id2label) == 0 {
		return nil, fmt.Errorf("tokencls config.json has no id2label map")
	}
	maxID := -1
	for k := range id2label {
		var id int
		if _, err := fmt.Sscanf(k, "%d", &id); err != nil {
			return nil, fmt.Errorf("invalid id2label key %q", k)
		}
		if id > maxID {
			maxID = id
		}
	}
	labels := make([]string, maxID+1)
	for i := range labels {
		labels[i] = "O"
	}
	for k, v := range id2label {
		var id int
		_, _ = fmt.Sscanf(k, "%d", &id)
		labels[id] = v
	}
	return labels, nil
}

// Name implements Engine.
func (tc *TokenClassifier) Name() string { return "tokencls" }

// Close releases the ONNX session and tensors.
func (tc *TokenClassifier) Close() {
	if tc.session != nil {
		_ = tc.session.Destroy()
	}
	for _, v := range []*ort.Tensor[int64]{tc.inputIDs, tc.attention, tc.typeIDs} {
		if v != nil {
			_ = v.Destroy()
		}
	}
	if tc.output != nil {
		_ = tc.output.Destroy()
	}
}

// Detect runs sliding-window inference and decodes BIO/BIOES labels.
func (tc *TokenClassifier) Detect(_ context.Context, doc *Document) (Result, error) {
	enc, err := tc.tokenizer.EncodeCharOffsets(
		tk.NewSingleEncodeInput(tk.NewInputSequence(doc.Text)), true)
	if err != nil {
		return Result{}, fmt.Errorf("tokenize: %w", err)
	}
	items := make([]tokenItem, 0, len(enc.Ids))
	runeCount := len([]rune(doc.Text))
	for i, id := range enc.Ids {
		if i < len(enc.SpecialTokenMask) && enc.SpecialTokenMask[i] == 1 {
			continue
		}
		if i >= len(enc.Offsets) || len(enc.Offsets[i]) < 2 || enc.Offsets[i][0] == enc.Offsets[i][1] {
			continue
		}
		startChar := enc.Offsets[i][0]
		endChar := tokenEndRune([]rune(doc.Text), startChar, enc.Offsets[i][1], enc.Tokens[i])
		if endChar > runeCount {
			endChar = runeCount
		}
		typ := 0
		if i < len(enc.TypeIds) {
			typ = enc.TypeIds[i]
		}
		items = append(items, tokenItem{
			startChar: startChar, endChar: endChar,
			id: id, typeID: typ,
		})
	}
	if len(items) == 0 {
		return Result{}, nil
	}

	var spans []Span
	for start := 0; start < len(items); start += tc.stride {
		end := start + tc.maxLen - 2
		if end > len(items) {
			end = len(items)
		}
		chunk := items[start:end]
		labels, err := tc.runWindow(chunk)
		if err != nil {
			return Result{}, err
		}
		spans = append(spans, decodeTokenSpans(doc, chunk, labels)...)
		if end >= len(items) {
			break
		}
	}
	return Result{Spans: MergeSpans(spans)}, nil
}

func (tc *TokenClassifier) runWindow(chunk []tokenItem) ([]string, error) {
	ids := tc.inputIDs.GetData()
	attn := tc.attention.GetData()
	for i := range ids {
		ids[i] = int64(tc.padID)
		attn[i] = 0
	}
	ids[0], attn[0] = int64(tc.clsID), 1
	for i, item := range chunk {
		ids[i+1] = int64(item.id)
		attn[i+1] = 1
	}
	last := len(chunk) + 1
	ids[last], attn[last] = int64(tc.sepID), 1
	if tc.hasTypeIDs && tc.typeIDs != nil {
		types := tc.typeIDs.GetData()
		for i := range types {
			types[i] = 0
		}
		for i, item := range chunk {
			types[i+1] = int64(item.typeID)
		}
	}
	if err := tc.session.Run(); err != nil {
		return nil, fmt.Errorf("tokencls inference: %w", err)
	}
	logits := tc.output.GetData()
	numLabels := len(tc.labels)
	labels := make([]string, len(chunk))
	for i := range chunk {
		base := (i + 1) * numLabels
		best, bestIdx := float32(-1e30), 0
		for j := 0; j < numLabels && base+j < len(logits); j++ {
			if logits[base+j] > best {
				best, bestIdx = logits[base+j], j
			}
		}
		labels[i] = tc.labels[bestIdx]
	}
	if len(chunk) > 0 {
		labels = viterbiLabels(logits, len(chunk), numLabels, tc.labels, tc.biases)
	}
	return labels, nil
}

// tokenTransitionBiases are the six BIOES transition categories used by the
// privyscope reference decoder. They trade recall against precision without
// retraining: high_recall encourages entering spans, which matters for
// precision-heavy checkpoints that rank O above B- on the first character.
type tokenTransitionBiases struct {
	outside, begin, single, inside, end, exit float32
}

var tokenOperatingPoints = map[string]tokenTransitionBiases{
	"balanced": {},
	"high_recall": {
		outside: -0.5, begin: 1.0, single: 1.0, inside: 0.8, end: 0.3, exit: -0.3,
	},
	"high_precision": {
		outside: 0.5, begin: -1.0, single: -1.0, inside: 0.2, end: 0.5, exit: 0.5,
	},
}

// resolveTokenBiases defaults to high_recall to match privyscope's zh-Hans
// preset; callers can pick another operating point or six raw biases.
func resolveTokenBiases(cfg *types.RedactTokenclsConfig) (tokenTransitionBiases, error) {
	if len(cfg.Biases) > 0 {
		if len(cfg.Biases) != 6 {
			return tokenTransitionBiases{}, fmt.Errorf("tokencls biases must have 6 values, got %d", len(cfg.Biases))
		}
		return tokenTransitionBiases{
			outside: float32(cfg.Biases[0]), begin: float32(cfg.Biases[1]),
			single: float32(cfg.Biases[2]), inside: float32(cfg.Biases[3]),
			end: float32(cfg.Biases[4]), exit: float32(cfg.Biases[5]),
		}, nil
	}
	name := strings.ToLower(strings.TrimSpace(cfg.OperatingPoint))
	if name == "" {
		name = "high_recall"
	}
	biases, ok := tokenOperatingPoints[name]
	if !ok {
		return tokenTransitionBiases{}, fmt.Errorf("unknown tokencls operating point %q (want high_recall, balanced or high_precision)", cfg.OperatingPoint)
	}
	return biases, nil
}

// viterbiLabels decodes a token path under BIO/BIOES legality. Independent
// argmax is not enough: a model can rank O above B-X on the first character of
// a name while E-X is the top label on the last character; the legal transition
// path plus category biases recovers the whole entity.
func viterbiLabels(logits []float32, seqLen, numLabels int, labels []string, biases tokenTransitionBiases) []string {
	if seqLen == 0 {
		return nil
	}
	emis := func(pos, label int) float32 {
		idx := (pos+1)*numLabels + label
		if idx < 0 || idx >= len(logits) {
			return -1e30
		}
		return logits[idx]
	}
	types := make([]string, numLabels)
	kinds := make([]byte, numLabels)
	bioes := false
	for i, label := range labels {
		kind, typ := splitTokenLabel(label)
		if kind == "" {
			kind = "O"
		}
		if kind == "E" || kind == "S" {
			bioes = true
		}
		kinds[i] = kind[0]
		types[i] = typ
	}
	transition := func(from, to int) float32 {
		fk, tk := kinds[from], kinds[to]
		if fk == 'B' || fk == 'I' {
			if tk == 'I' {
				return biases.inside
			}
			return biases.end
		}
		// O/E/S: start fresh.
		switch tk {
		case 'B':
			return biases.begin
		case 'S':
			return biases.single
		case 'O':
			if fk == 'E' || fk == 'S' {
				return biases.exit
			}
			return biases.outside
		}
		return 0
	}
	allowed := func(from, to int) bool {
		fk, tk := kinds[from], kinds[to]
		if fk == 'O' {
			return tk == 'O' || tk == 'B' || tk == 'S'
		}
		if fk == 'B' || fk == 'I' {
			if tk == 'I' || tk == 'E' {
				return types[from] == types[to]
			}
			// BIO models have no E tag; O closes the span.
			return !bioes && tk == 'O'
		}
		// E/S close an entity; the next token starts fresh.
		return tk == 'O' || tk == 'B' || tk == 'S'
	}
	startAllowed := func(j int) bool {
		k := kinds[j]
		return k == 'O' || k == 'B' || k == 'S'
	}
	endAllowed := func(j int) bool {
		k := kinds[j]
		return k == 'O' || k == 'E' || k == 'S' || (!bioes && k == 'I')
	}
	prev := make([]float32, numLabels)
	curr := make([]float32, numLabels)
	back := make([][]int, seqLen)
	for i := range back {
		back[i] = make([]int, numLabels)
	}
	for j := 0; j < numLabels; j++ {
		if startAllowed(j) {
			prev[j] = emis(0, j)
		} else {
			prev[j] = -1e30
		}
	}
	for pos := 1; pos < seqLen; pos++ {
		for j := 0; j < numLabels; j++ {
			best, bestFrom := float32(-1e30), -1
			for k := 0; k < numLabels; k++ {
				if prev[k] <= -1e29 || !allowed(k, j) {
					continue
				}
				if score := prev[k] + emis(pos, j) + transition(k, j); score > best {
					best, bestFrom = score, k
				}
			}
			curr[j], back[pos][j] = best, bestFrom
		}
		prev, curr = curr, prev
	}
	best, bestIdx := float32(-1e30), -1
	for j := 0; j < numLabels; j++ {
		if prev[j] > best && endAllowed(j) {
			best, bestIdx = prev[j], j
		}
	}
	if bestIdx < 0 {
		// Fall back to independent argmax when no legal path exists.
		out := make([]string, seqLen)
		for pos := 0; pos < seqLen; pos++ {
			bestLabel, bestScore := 0, float32(-1e30)
			for j := 0; j < numLabels; j++ {
				if s := emis(pos, j); s > bestScore {
					bestScore, bestLabel = s, j
				}
			}
			out[pos] = labels[bestLabel]
		}
		return out
	}
	path := make([]int, seqLen)
	path[seqLen-1] = bestIdx
	for pos := seqLen - 1; pos > 0; pos-- {
		path[pos-1] = back[pos][path[pos]]
	}
	out := make([]string, seqLen)
	for i, idx := range path {
		if idx >= 0 && idx < len(labels) {
			out[i] = labels[idx]
		} else {
			out[i] = "O"
		}
	}
	return out
}

// supportedModelType is the allowlist of entity types a token-classification
// model may emit. Unknown labels are ignored so third-party NER models cannot
// turn MISC/EVENT spans into redaction boxes.
func supportedModelType(typ string) bool {
	switch strings.ToUpper(strings.TrimSpace(typ)) {
	case "PERSON", "ADDRESS", "ORG", "PHONE", "EMAIL",
		"ID_CARD", "PASSPORT", "TRAVEL_PERMIT", "SSN", "AGE", "ETHNICITY",
		"NATIONAL_ID", "DRIVER_LICENSE",
		"BANK_CARD", "BANK_ACCOUNT", "USCC", "TAX_ID", "DATE", "AMOUNT",
		"CASE_NO", "CONTRACT_NO", "DOC_NO", "POLICY_NO", "MEDICAL_NO",
		"PLATE", "VIN", "ENGINE_NO", "CERT_NO", "BUSINESS_REG_NO",
		"PROJECT_NO", "INVOICE_NO", "PROPERTY_CERT_NO", "PERMIT_NO",
		"COPYRIGHT_NO", "WORK", "LAWYER_NO", "POSTCODE", "WECHAT", "QQ",
		"CRYPTO", "IP", "URL", "SECRET", "CREDENTIAL", "TOKEN", "PASSWORD",
		"USERNAME", "HOST", "COOKIE", "DB_URL", "API_KEY", "PRIVATE_KEY",
		"STOCK_CODE", "MAC", "EMPLOYEE_NO":
		return true
	}
	return false
}

func decodeTokenSpans(doc *Document, items []tokenItem, labels []string) []Span {
	var spans []Span
	openStart, openEnd := -1, -1
	openType := ""
	closeSpan := func() {
		if openType != "" && openStart >= 0 {
			start, end := doc.ByteOffset(openStart), doc.ByteOffset(openEnd)
			if start < end {
				spans = append(spans, Span{
					Start: start, End: end, Type: openType,
					Text: doc.Text[start:end], Rule: "tokencls",
					Source: SourceModel, Score: 0.9, Priority: 55,
				})
			}
		}
		openStart, openEnd, openType = -1, -1, ""
	}
	for i, item := range items {
		label := "O"
		if i < len(labels) {
			label = labels[i]
		}
		tag, ent := splitTokenLabel(label)
		if tag == "" {
			closeSpan()
			continue
		}
		typ := CanonicalType(ent)
		if !supportedModelType(typ) {
			// Third-party models (WikiAnn and friends) also emit MISC/EVENT and
			// other labels we do not redact. Ignore them instead of masking
			// arbitrary text.
			closeSpan()
			continue
		}
		switch tag {
		case "B":
			closeSpan()
			openStart, openEnd, openType = item.startChar, item.endChar, typ
		case "S":
			closeSpan()
			if typ != "" {
				start, end := doc.ByteOffset(item.startChar), doc.ByteOffset(item.endChar)
				if start < end {
					spans = append(spans, Span{
						Start: start, End: end, Type: typ,
						Text: doc.Text[start:end], Rule: "tokencls",
						Source: SourceModel, Score: 0.9, Priority: 55,
					})
				}
			}
		case "I":
			if openType == typ {
				openEnd = item.endChar
			} else {
				closeSpan()
				openStart, openEnd, openType = item.startChar, item.endChar, typ
			}
		case "E":
			if openType == typ {
				openEnd = item.endChar
				closeSpan()
			} else {
				closeSpan()
				start, end := doc.ByteOffset(item.startChar), doc.ByteOffset(item.endChar)
				if start < end {
					spans = append(spans, Span{
						Start: start, End: end, Type: typ,
						Text: doc.Text[start:end], Rule: "tokencls",
						Source: SourceModel, Score: 0.9, Priority: 55,
					})
				}
			}
		}
	}
	closeSpan()
	sort.Slice(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	return extendAddressSpans(doc, dedupeSpans(spans))
}

func splitTokenLabel(label string) (string, string) {
	up := strings.ToUpper(strings.TrimSpace(label))
	for _, tag := range []string{"B-", "I-", "E-", "S-"} {
		if strings.HasPrefix(up, tag) {
			return tag[:1], label[len(tag):]
		}
	}
	if up == "O" || up == "" {
		return "", ""
	}
	// BIO-only models sometimes emit a bare entity label; treat it as B.
	return "B", label
}
