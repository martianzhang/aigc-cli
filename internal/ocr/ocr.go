// Package ocr provides offline text detection and recognition using ONNX Runtime.
// Detection uses DBNet (PP-OCRv5), recognition uses CRNN/SVTR with CTC decoding.
// Zero CGO dependency — all inference goes through pure-onnx.
package ocr

import (
	"bufio"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ort "github.com/amikos-tech/pure-onnx/ort"
)

// OCRLine holds a single recognized text line.
type OCRLine struct {
	Text       string    `json:"text"`
	BBox       [4][2]int `json:"bbox"` // four-point polygon: [top-left, top-right, bottom-right, bottom-left]
	Confidence float32   `json:"confidence"`
}

// OCRPage holds all lines found on one page.
type OCRPage struct {
	Page  int       `json:"page"`
	Lines []OCRLine `json:"lines"`
}

// OCRResult is the complete OCR output for a document.
type OCRResult struct {
	Pages []OCRPage `json:"pages"`
	Text  string    `json:"text"` // plain-text concatenation with newlines
}

const (
	// DetMaxSide is the max image dimension for detection (maintains aspect ratio).
	DetMaxSide = 960
	// DetInputSize is the padded size fed into the det model (must be ≥ DetMaxSide).
	DetInputSize = 960
	// DetChannels is the number of input channels for detection.
	DetChannels = 3
	// DetDownsample is the total stride of the DBNet backbone.
	// PP-OCRv4 det outputs a full-resolution probability map (1:1 with padded input).
	DetDownsample = 1

	// RecHeight is the fixed height for recognition input.
	RecHeight = 48
	// RecMaxWidth is the maximum padded width for recognition.
	RecMaxWidth = 960
	// RecVocabSize is the vocabulary size for Chinese recognition (PP-OCR key).
	RecVocabSize = 6625

	// DetInputName is the expected detection model input tensor name.
	DetInputName = "x"
	// DetOutputName is the expected detection model output tensor name.
	DetOutputName = "sigmoid_0.tmp_0"
	// RecInputName is the expected recognition model input tensor name.
	RecInputName = "x"
	// RecOutputName is the expected recognition model output tensor name.
	RecOutputName = "softmax_11.tmp_0"
	// EnRecOutputName is the English recognition model output tensor name.
	EnRecOutputName = "softmax_2.tmp_0"
	// EnRecVocabSize is the vocabulary size for English recognition.
	EnRecVocabSize = 97
	// ClsInputName is the expected direction classifier input tensor name.
	ClsInputName = "x"
	// ClsOutputName is the expected direction classifier output tensor name.
	ClsOutputName = "save_infer_model/scale_0.tmp_1"
)

// Engine manages ONNX sessions for the OCR pipeline (det + rec + cls).
type Engine struct {
	libPath  string
	detModel string
	recModel string
	clsModel string // optional direction classifier
	dictPath string // optional dictionary file (dict.txt)
	enModel  string // optional English recognition model
	enDict   string // optional English dictionary
	Lang     string // language mode: "auto", "zh", "en" (extensible)

	dict       []string // loaded dictionary (index → character)
	enDictList []string // English dictionary

	recVocabSize int    // vocabulary size for the loaded rec model
	recOutName   string // output tensor name for the loaded rec model

	det   *ort.AdvancedSession
	rec   *ort.AdvancedSession
	enRec *ort.AdvancedSession // English recognition session
	cls   *ort.AdvancedSession
	detIn *ort.Tensor[float32]
	recIn *ort.Tensor[float32]
	enIn  *ort.Tensor[float32]
	clsIn *ort.Tensor[float32]

	detOut *ort.Tensor[float32]
	recOut *ort.Tensor[float32]
	enOut  *ort.Tensor[float32]
	clsOut *ort.Tensor[float32]
}

// NewEngine creates an OCR engine, loading ONNX Runtime and all model files.
func NewEngine(libPath, detModel, recModel, clsModel, dictPath string, recVocabSize int, recOutName string, enModel, enDict, lang string) (*Engine, error) {
	e := &Engine{
		libPath:      libPath,
		detModel:     detModel,
		recModel:     recModel,
		clsModel:     clsModel,
		dictPath:     dictPath,
		recVocabSize: recVocabSize,
		recOutName:   recOutName,
		enModel:      enModel,
		enDict:       enDict,
		Lang:         lang,
	}
	if err := e.init(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Engine) init() error {
	// Load dictionary if provided
	if e.dictPath != "" {
		dict, err := loadDict(e.dictPath)
		if err != nil {
			return fmt.Errorf("load dictionary: %w", err)
		}
		e.dict = dict
	}
	// Load English dictionary if provided
	if e.enDict != "" {
		dict, err := loadDict(e.enDict)
		if err != nil {
			return fmt.Errorf("load english dictionary: %w", err)
		}
		e.enDictList = dict
	}

	if err := ort.SetSharedLibraryPath(e.libPath); err != nil {
		return fmt.Errorf("set library path: %w", err)
	}
	_ = ort.SetLogLevel(ort.LoggingLevelError)
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("initialize environment: %w", err)
	}

	// Detection session
	if err := e.initDet(); err != nil {
		ort.DestroyEnvironment()
		return fmt.Errorf("detection init: %w", err)
	}
	// Recognition session
	if err := e.initRec(); err != nil {
		e.cleanupDet()
		ort.DestroyEnvironment()
		return fmt.Errorf("recognition init: %w", err)
	}
	// Optional English recognition session
	if e.enModel != "" {
		if _, err := os.Stat(e.enModel); err == nil {
			if err := e.initEnRec(); err != nil {
				e.cleanupRec()
				e.cleanupDet()
				ort.DestroyEnvironment()
				return fmt.Errorf("english rec init: %w", err)
			}
		}
	}
	// Optional direction classifier
	if e.clsModel != "" {
		if err := e.initCls(); err != nil {
			e.cleanupEnRec()
			e.cleanupRec()
			e.cleanupDet()
			ort.DestroyEnvironment()
			return fmt.Errorf("classifier init: %w", err)
		}
	}
	return nil
}

func (e *Engine) initDet() error {
	if _, err := os.Stat(e.detModel); err != nil {
		return fmt.Errorf("detection model not found: %w", err)
	}
	// Input: [1, 3, 960, 960], Output: [1, 1, 960, 960]
	inputShape := ort.NewShape(1, DetChannels, DetInputSize, DetInputSize)
	totalIn := 1 * DetChannels * DetInputSize * DetInputSize
	inputData := make([]float32, totalIn)
	var err error
	e.detIn, err = ort.NewTensor(inputShape, inputData)
	if err != nil {
		return fmt.Errorf("create det input tensor: %w", err)
	}

	outputShape := ort.NewShape(1, 1, DetInputSize, DetInputSize)
	totalOut := 1 * 1 * DetInputSize * DetInputSize
	outputData := make([]float32, totalOut)
	e.detOut, err = ort.NewTensor(outputShape, outputData)
	if err != nil {
		e.detIn.Destroy()
		return fmt.Errorf("create det output tensor: %w", err)
	}

	e.det, err = ort.NewAdvancedSession(
		e.detModel,
		[]string{DetInputName},
		[]string{DetOutputName},
		[]ort.Value{e.detIn},
		[]ort.Value{e.detOut},
		nil,
	)
	if err != nil {
		e.detOut.Destroy()
		e.detIn.Destroy()
		return fmt.Errorf("create det session: %w", err)
	}
	return nil
}

func (e *Engine) initRec() error {
	if _, err := os.Stat(e.recModel); err != nil {
		return fmt.Errorf("recognition model not found: %w", err)
	}
	// Input: [1, 3, 48, 960], Output: [1, T, VocabSize] where T = W/8
	inputShape := ort.NewShape(1, DetChannels, RecHeight, RecMaxWidth)
	totalIn := 1 * DetChannels * RecHeight * RecMaxWidth
	inputData := make([]float32, totalIn)
	var err error
	e.recIn, err = ort.NewTensor(inputShape, inputData)
	if err != nil {
		return fmt.Errorf("create rec input tensor: %w", err)
	}

	recTimesteps := RecMaxWidth / 8
	outputShape := ort.NewShape(1, int64(recTimesteps), int64(e.recVocabSize))
	totalOut := 1 * recTimesteps * e.recVocabSize
	outputData := make([]float32, totalOut)
	e.recOut, err = ort.NewTensor(outputShape, outputData)
	if err != nil {
		e.recIn.Destroy()
		return fmt.Errorf("create rec output tensor: %w", err)
	}

	e.rec, err = ort.NewAdvancedSession(
		e.recModel,
		[]string{RecInputName},
		[]string{e.recOutName},
		[]ort.Value{e.recIn},
		[]ort.Value{e.recOut},
		nil,
	)
	if err != nil {
		e.recOut.Destroy()
		e.recIn.Destroy()
		return fmt.Errorf("create rec session: %w", err)
	}
	return nil
}

func (e *Engine) initEnRec() error {
	if _, err := os.Stat(e.enModel); err != nil {
		return fmt.Errorf("english model not found: %w", err)
	}
	// Input: [1, 3, 48, 960], Output: [1, T, 97]
	inputShape := ort.NewShape(1, DetChannels, RecHeight, RecMaxWidth)
	totalIn := 1 * DetChannels * RecHeight * RecMaxWidth
	inputData := make([]float32, totalIn)
	var err error
	e.enIn, err = ort.NewTensor(inputShape, inputData)
	if err != nil {
		return fmt.Errorf("create en input tensor: %w", err)
	}

	recTimesteps := RecMaxWidth / 8
	outputShape := ort.NewShape(1, int64(recTimesteps), EnRecVocabSize)
	totalOut := 1 * recTimesteps * EnRecVocabSize
	outputData := make([]float32, totalOut)
	e.enOut, err = ort.NewTensor(outputShape, outputData)
	if err != nil {
		e.enIn.Destroy()
		return fmt.Errorf("create en output tensor: %w", err)
	}

	e.enRec, err = ort.NewAdvancedSession(
		e.enModel,
		[]string{RecInputName},
		[]string{EnRecOutputName},
		[]ort.Value{e.enIn},
		[]ort.Value{e.enOut},
		nil,
	)
	if err != nil {
		e.enOut.Destroy()
		e.enIn.Destroy()
		return fmt.Errorf("create en rec session: %w", err)
	}
	return nil
}

func (e *Engine) initCls() error {
	if _, err := os.Stat(e.clsModel); err != nil {
		return fmt.Errorf("classifier model not found: %w", err)
	}
	// Input: [1, 3, 48, 192], Output: [1, 2]
	inputShape := ort.NewShape(1, DetChannels, 48, 192)
	totalIn := 1 * DetChannels * 48 * 192
	inputData := make([]float32, totalIn)
	var err error
	e.clsIn, err = ort.NewTensor(inputShape, inputData)
	if err != nil {
		return fmt.Errorf("create cls input tensor: %w", err)
	}

	outputShape := ort.NewShape(1, 2)
	outputData := make([]float32, 2)
	e.clsOut, err = ort.NewTensor(outputShape, outputData)
	if err != nil {
		e.clsIn.Destroy()
		return fmt.Errorf("create cls output tensor: %w", err)
	}

	e.cls, err = ort.NewAdvancedSession(
		e.clsModel,
		[]string{ClsInputName},
		[]string{ClsOutputName},
		[]ort.Value{e.clsIn},
		[]ort.Value{e.clsOut},
		nil,
	)
	if err != nil {
		e.clsOut.Destroy()
		e.clsIn.Destroy()
		return fmt.Errorf("create cls session: %w", err)
	}
	return nil
}

func (e *Engine) cleanupDet() {
	if e.det != nil {
		e.det.Destroy()
	}
	if e.detOut != nil {
		e.detOut.Destroy()
	}
	if e.detIn != nil {
		e.detIn.Destroy()
	}
}

func (e *Engine) cleanupRec() {
	if e.rec != nil {
		e.rec.Destroy()
	}
	if e.recOut != nil {
		e.recOut.Destroy()
	}
	if e.recIn != nil {
		e.recIn.Destroy()
	}
}

func (e *Engine) cleanupEnRec() {
	if e.enRec != nil {
		e.enRec.Destroy()
	}
	if e.enOut != nil {
		e.enOut.Destroy()
	}
	if e.enIn != nil {
		e.enIn.Destroy()
	}
}

func (e *Engine) cleanupCls() {
	if e.cls != nil {
		e.cls.Destroy()
	}
	if e.clsOut != nil {
		e.clsOut.Destroy()
	}
	if e.clsIn != nil {
		e.clsIn.Destroy()
	}
}

// Close releases all ONNX Runtime resources.
func (e *Engine) Close() {
	e.cleanupCls()
	e.cleanupEnRec()
	e.cleanupRec()
	e.cleanupDet()
	ort.DestroyEnvironment()
}

// DefaultLibPath returns the OS-appropriate ONNX Runtime shared library path.
func DefaultLibPath(modelsDir string) (string, error) {
	candidates := []string{
		filepath.Join(modelsDir, "libonnxruntime_gpu.dylib"),
		filepath.Join(modelsDir, "libonnxruntime_gpu.so"),
		filepath.Join(modelsDir, "onnxruntime_gpu.dll"),
		filepath.Join(modelsDir, "libonnxruntime.dylib"),
		filepath.Join(modelsDir, "libonnxruntime.so"),
		filepath.Join(modelsDir, "onnxruntime.dll"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("ONNX Runtime library not found in %s", modelsDir)
}

// DefaultDetModelPath returns the default detection model path.
func DefaultDetModelPath(modelsDir string) string {
	return filepath.Join(modelsDir, "ch_PP-OCRv4_det_infer.onnx")
}

// DefaultRecModelPath returns the default recognition model path (Chinese).
func DefaultRecModelPath(modelsDir string) string {
	return filepath.Join(modelsDir, "ch_PP-OCRv4_rec_infer.onnx")
}

// DefaultEnRecModelPath returns the default English recognition model path.
func DefaultEnRecModelPath(modelsDir string) string {
	return filepath.Join(modelsDir, "rec_en_PP-OCRv3_infer.onnx")
}

// DefaultClsModelPath returns the default direction classifier path.
func DefaultClsModelPath(modelsDir string) string {
	return filepath.Join(modelsDir, "ch_ppocr_mobile_v2.0_cls_infer.onnx")
}

// DefaultDictPath returns the default Chinese dictionary path.
func DefaultDictPath(modelsDir string) string {
	return filepath.Join(modelsDir, "dict_zh.txt")
}

// DefaultEnDictPath returns the default English dictionary path.
func DefaultEnDictPath(modelsDir string) string {
	return filepath.Join(modelsDir, "dict_en.txt")
}

// loadDict loads a PP-OCR dictionary file where each line is one character.
// Line number (1-based) = character index in the model's output.
func loadDict(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var chars []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		chars = append(chars, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read dict: %w", err)
	}
	if len(chars) == 0 {
		return nil, fmt.Errorf("empty dictionary: %s", path)
	}
	return chars, nil
}

// sortBoxesReadingOrder sorts text boxes in natural reading order:
// top-to-bottom, left-to-right. Two boxes are considered same row when
// their vertical center lines fall within each other's height range.
func sortBoxesReadingOrder(boxes [][4][2]int) {
	// Compute row-grouped centers
	type idxBox struct {
		idx    int
		cy     int
		cx     int
		height int
	}
	items := make([]idxBox, len(boxes))
	for i, b := range boxes {
		minY, maxY := b[0][1], b[0][1]
		minX, maxX := b[0][0], b[0][0]
		for _, p := range b[1:] {
			if p[0] < minX {
				minX = p[0]
			}
			if p[0] > maxX {
				maxX = p[0]
			}
			if p[1] < minY {
				minY = p[1]
			}
			if p[1] > maxY {
				maxY = p[1]
			}
		}
		items[i] = idxBox{
			idx:    i,
			cy:     (minY + maxY) / 2,
			cx:     (minX + maxX) / 2,
			height: maxY - minY,
		}
	}
	// Sort by row (y) then column (x)
	sort.Slice(items, func(i, j int) bool {
		// Same row if vertical distance < half the shorter box height
		overlap := items[i].height + items[j].height
		dist := items[i].cy - items[j].cy
		if dist < 0 {
			dist = -dist
		}
		if dist*2 < overlap {
			return items[i].cx < items[j].cx
		}
		return items[i].cy < items[j].cy
	})
	// Reorder boxes in-place
	sorted := make([][4][2]int, len(boxes))
	for i, item := range items {
		sorted[i] = boxes[item.idx]
	}
	copy(boxes, sorted)
}

// expandBox expands a text region box outward by a ratio, giving the
// recognition model some surrounding context (important for edge pixels).
func expandBox(box [4][2]int, imgW, imgH int, ratio float64) [4][2]int {
	minX, maxX := box[0][0], box[0][0]
	minY, maxY := box[0][1], box[0][1]
	for _, p := range box[1:] {
		if p[0] < minX {
			minX = p[0]
		}
		if p[0] > maxX {
			maxX = p[0]
		}
		if p[1] < minY {
			minY = p[1]
		}
		if p[1] > maxY {
			maxY = p[1]
		}
	}
	w := maxX - minX
	h := maxY - minY
	padX := int(float64(w) * ratio)
	padY := int(float64(h) * ratio)

	// Clamp to image bounds
	exMinX := maxInt(minX-padX, 0)
	exMinY := maxInt(minY-padY, 0)
	exMaxX := minInt(maxX+padX, imgW)
	exMaxY := minInt(maxY+padY, imgH)

	return [4][2]int{
		{exMinX, exMinY},
		{exMaxX, exMinY},
		{exMaxX, exMaxY},
		{exMinX, exMaxY},
	}
}

// isCJK reports whether a rune is a CJK character (Chinese, Japanese, Korean).
func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3040 && r <= 0x30FF) || // Hiragana + Katakana
		(r >= 0xAC00 && r <= 0xD7AF) || // Hangul
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0xFF00 && r <= 0xFFEF) // Fullwidth forms
}

// groupLinesIntoParagraphs groups recognized lines into paragraphs based on
// their vertical position and indentation. Uses a simplified version of
// Umi-OCR's paragraph parsing approach.
func groupLinesIntoParagraphs(lines []OCRLine) string {
	if len(lines) == 0 {
		return ""
	}

	// Group into rows: within each row, blocks are horizontally arranged.
	// Two blocks are in the same row if their vertical centers overlap.
	type rowBlock struct {
		line OCRLine
		cx   int
	}
	var rows [][]rowBlock
	var rowTops []int

	for _, line := range lines {
		box := line.BBox
		minY, maxY := box[0][1], box[0][1]
		for _, p := range box[1:] {
			if p[1] < minY {
				minY = p[1]
			}
			if p[1] > maxY {
				maxY = p[1]
			}
		}
		cy := (minY + maxY) / 2
		height := maxY - minY
		placed := false

		for ri := range rows {
			if len(rows[ri]) > 0 {
				// Check vertical overlap
				firstBox := rows[ri][0].line.BBox
				rowMinY, rowMaxY := firstBox[0][1], firstBox[0][1]
				for _, p := range firstBox[1:] {
					if p[1] < rowMinY {
						rowMinY = p[1]
					}
					if p[1] > rowMaxY {
						rowMaxY = p[1]
					}
				}
				rowHeight := rowMaxY - rowMinY
				overlap := height + rowHeight
				dist := cy - (rowMinY+rowMaxY)/2
				if dist < 0 {
					dist = -dist
				}
				if dist*2 < overlap {
					rows[ri] = append(rows[ri], rowBlock{line, (box[0][0] + box[2][0]) / 2})
					placed = true
					break
				}
			}
		}
		if !placed {
			rows = append(rows, []rowBlock{{line, (box[0][0] + box[2][0]) / 2}})
			rowTops = append(rowTops, cy)
		}
	}

	// Sort rows top-to-bottom, then each row left-to-right
	sort.SliceStable(rows, func(i, j int) bool {
		return rowTops[i] < rowTops[j]
	})
	for ri := range rows {
		sort.Slice(rows[ri], func(i, j int) bool {
			return rows[ri][i].cx < rows[ri][j].cx
		})
	}

	// Build output with separators
	var result strings.Builder
	for ri, row := range rows {
		for bi, blk := range row {
			if bi > 0 {
				// Check horizontal gap to detect column boundaries
				prevBox := row[bi-1].line.BBox
				prevRight := prevBox[1][0]      // top-right x
				currLeft := blk.line.BBox[0][0] // top-left x
				gap := currLeft - prevRight
				// Use a minimum gap threshold of 20px to detect column breaks
				if gap > 20 {
					result.WriteString("\n")
				} else {
					result.WriteString(" ")
				}
			}
			result.WriteString(blk.line.Text)
		}
		if ri < len(rows)-1 {
			result.WriteString("\n")
		}
	}
	return cjkLatinSpacing(result.String())
}

// isEnglishText checks whether OCR output is predominantly English.
// Uses a threshold: if >70% of non-space characters are ASCII, treat as English.
func isEnglishText(lines []OCRLine) bool {
	var ascii, cjk int
	for _, l := range lines {
		for _, r := range l.Text {
			if r == ' ' || r == '\u6781' {
				continue
			}
			if r >= 0x4E00 && r <= 0x9FFF {
				cjk++
			} else if r <= 0x7F {
				ascii++
			}
		}
	}
	total := ascii + cjk
	if total == 0 {
		return false
	}
	return float64(ascii)/float64(total) > 0.7
}

// fixEnglishOCRErrors corrects common digit-letter confusions in English OCR output.
// The English model (PP-OCRv3) sometimes confuses visually similar characters
// like 1/l, 0/O in certain fonts.
func fixEnglishOCRErrors(text string) string {
	runes := []rune(text)
	for i, r := range runes {
		switch r {
		case '1':
			if i > 0 && i < len(runes)-1 &&
				isLetter(runes[i-1]) && isLetter(runes[i+1]) {
				runes[i] = 'l'
			}
			if i == len(runes)-1 && i > 0 && isLetter(runes[i-1]) {
				runes[i] = 'l'
			}
		case '0':
			if i > 0 && i < len(runes)-1 &&
				isLetter(runes[i-1]) && isLetter(runes[i+1]) {
				runes[i] = 'O'
			}
			if i == len(runes)-1 && i > 0 && isLetter(runes[i-1]) {
				runes[i] = 'o'
			}
		}
	}
	return string(runes)
}

// cjkLatinSpacing inserts spaces between CJK and Latin/digit characters
// for better readability in mixed text. E.g., "AI融资" → "AI 融资".
func cjkLatinSpacing(text string) string {
	runes := []rune(text)
	var result []rune
	for i, r := range runes {
		if i > 0 {
			prev := runes[i-1]
			switch {
			case isCJKOpenParen(prev) || isCJKCloseParen(r):
				// No space after open paren or before close paren
			case isCJK(r) && (isLetter(prev) || isDigit(prev)):
				result = append(result, ' ')
			case isCJK(prev) && (isLetter(r) || isDigit(r)):
				result = append(result, ' ')
			}
		}
		result = append(result, r)
	}
	return string(result)
}

func isCJKOpenParen(r rune) bool {
	return r == '\uFF08' || r == '\u300A' || r == '\u300C' || r == '\u3010'
}

func isCJKCloseParen(r rune) bool {
	return r == '\uFF09' || r == '\u300B' || r == '\u300D' || r == '\u3011'
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// isLetter checks if a rune is an ASCII letter.
func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// postProcessLine cleans up OCR output.
// The Chinese PP-OCR model has no space (index 0 is blank), uses CJK filler,
// and sometimes outputs wrong characters for English screenshots.
func postProcessLine(text string) string {
	// Replace "极" with space (Chinese model uses it as word-boundary filler)
	text = strings.ReplaceAll(text, "\u6781", " ")

	// Collapse multiple spaces
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	text = strings.TrimSpace(text)

	// Insert space after punctuation followed by a letter (but NOT decimal points)
	var punctFixed strings.Builder
	runes := []rune(text)
	for i, r := range runes {
		punctFixed.WriteRune(r)
		if i < len(runes)-1 {
			next := runes[i+1]
			switch r {
			case '.', ',':
				// Don't add space before digits (decimal points)
				if next >= '0' && next <= '9' {
					continue
				}
				// Don't add space before closing punctuation
				if next == ')' || next == ']' || next == '"' || next == '\'' {
					continue
				}
				// Add space before letters
				if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') {
					punctFixed.WriteRune(' ')
				}
			case ':', ';', '?', '!':
				// Always add space after these (for both letters and digits)
				punctFixed.WriteRune(' ')
			}
		}
	}
	text = punctFixed.String()

	// Insert space before uppercase following lowercase (merged CamelCase)
	var spaced strings.Builder
	runes2 := []rune(text)
	for i, r := range runes2 {
		if i > 0 && r >= 'A' && r <= 'Z' && runes2[i-1] >= 'a' && runes2[i-1] <= 'z' {
			spaced.WriteRune(' ')
		}
		// Insert space between digit and letter (e.g., "2hours" → "2 hours")
		if i > 0 && isDigit(r) && isLetter(runes2[i-1]) {
			spaced.WriteRune(' ')
		}
		if i > 0 && isLetter(r) && isDigit(runes2[i-1]) {
			spaced.WriteRune(' ')
		}
		spaced.WriteRune(r)
	}
	text = spaced.String()

	// Collapse multiple spaces from the additions above
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	// Correct common model-level typos
	text = fixCommonOCRErrors(text)

	return text
}

// fixCommonOCRErrors corrects known PP-OCR recognition errors on English text.
// fixCommonOCRErrors corrects known PP-OCR character-level confusions.
// Uses word-level matching (not substring) to avoid false positives.
func fixCommonOCRErrors(text string) string {
	type fix struct{ old, new string }
	// Common PP-OCR confusions on the Chinese+English model when run on English text.
	letters := []fix{
		{"tor", "for"},                   // t→f
		{"fhe", "the"},                   // f→t
		{"evervone", "everyone"},         // v→y
		{"Onen", "Open"},                 // n→p
		{"davs", "days"},                 // v→y
		{"hour1", "hour"},                // 1→ (no char)
		{"ho1r", "hour"},                 // 1→u
		{"1n", "in"},                     // 1→i
		{"subscribtion", "subscription"}, // missing p
		{"usagelimit", "usage limit"},
		{"God", "Go"}, // partial word
	}
	words := strings.Fields(text)
	for i, w := range words {
		for _, f := range letters {
			if strings.EqualFold(w, f.old) {
				words[i] = f.new
				break
			}
		}
	}
	return strings.Join(words, " ")
}

// Scan runs OCR on a single image and returns the recognized text.
func (e *Engine) Scan(img image.Image) (*OCRResult, error) {
	// Step 1: Text detection
	boxes, err := e.Detect(img)
	if err != nil {
		return nil, fmt.Errorf("detection failed: %w", err)
	}

	if len(boxes) == 0 {
		return &OCRResult{
			Pages: []OCRPage{{Page: 0, Lines: nil}},
			Text:  "",
		}, nil
	}

	// Step 2: Sort boxes in reading order before recognition
	sortBoxesReadingOrder(boxes)

	// Step 3: Recognize each text region (Chinese model first)
	var lines []OCRLine
	bounds := img.Bounds()
	for _, box := range boxes {
		expanded := expandBox(box, bounds.Max.X, bounds.Max.Y, 0.10)
		line, err := e.Recognize(img, expanded)
		if err != nil {
			continue
		}
		line.Text = postProcessLine(line.Text)
		lines = append(lines, *line)
	}

	// Auto-detect language: if English model is available and no CJK in output,
	// re-run with English model for much better English accuracy.
	// Auto-detect language: if English model is available and no CJK in output,
	// re-run with English model for much better English accuracy.
	useEN := e.enRec != nil && (e.Lang == "en" || (e.Lang == "auto" && isEnglishText(lines)))
	if useEN {
		var enLines []OCRLine
		for _, box := range boxes {
			expanded := expandBox(box, bounds.Max.X, bounds.Max.Y, 0.10)
			line, err := e.RecognizeEN(img, expanded)
			if err != nil {
				continue
			}
			enLines = append(enLines, *line)
		}
		if len(enLines) > 0 {
			lines = enLines
		}
	}

	// Build result with paragraph grouping
	text := groupLinesIntoParagraphs(lines)

	return &OCRResult{
		Pages: []OCRPage{{
			Page:  0,
			Lines: lines,
		}},
		Text: text,
	}, nil
}

// ScanFile opens an image file and runs OCR.
func (e *Engine) ScanFile(path string) (*OCRResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return e.Scan(img)
}
