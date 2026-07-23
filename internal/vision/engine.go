package vision

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ort "github.com/amikos-tech/pure-onnx/ort"
)

// Engine wraps the four Florence-2 ONNX sub-models into a single inference engine.
type Engine struct {
	libPath   string
	modelsDir string
	variant   string

	visionSession  *ort.AdvancedSession
	embedSession   *ort.AdvancedSession
	encoderSession *ort.AdvancedSession
	decoderSession *ort.AdvancedSession

	tokenizer *Tokenizer
	sampler   *Sampler

	visionPixels *ort.Tensor[float32]
	visionOutput *ort.Tensor[float32]

	embedInput  *ort.Tensor[int64]
	embedOutput *ort.Tensor[float32]

	encInputEmbeds   *ort.Tensor[float32]
	encAttentionMask *ort.Tensor[int64]
	encOutput        *ort.Tensor[float32]

	// Decoder: pre-allocated at max size, reused each step.
	decInputEmbeds *ort.Tensor[float32]
	decEncStates   *ort.Tensor[float32]
	decEncAttnMask *ort.Tensor[int64]
	decOutput      *ort.Tensor[float32]

	initialized bool
}

type EngineConfig struct {
	ModelsDir   string
	Variant     string
	LibPath     string
	Tokenizer   *Tokenizer
	MaxTokens   int
	Temperature float64
	TopK        int
}

func DefaultModelsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aigc-cli", "models")
}

func VariantDir(modelsDir, variant string) string {
	return filepath.Join(modelsDir, "vision", variant)
}

// modelPath returns the path to a model file within the variant directory.
func modelPath(modelsDir, variant, filename string) string {
	return filepath.Join(modelsDir, "vision", variant, filename)
}

// NewEngine creates and initializes the vision engine with all four ONNX sessions.
func NewEngine(cfg *EngineConfig) (*Engine, error) {
	e := &Engine{
		libPath:   cfg.LibPath,
		modelsDir: cfg.ModelsDir,
		variant:   cfg.Variant,
		tokenizer: cfg.Tokenizer,
		sampler:   NewSampler(cfg.Temperature, cfg.TopK),
	}
	if e.variant == "" {
		e.variant = DefaultModelVariant
	}
	if err := e.init(); err != nil {
		return nil, err
	}
	return e, nil
}

// newSession creates an ONNX session.
func (e *Engine) newSession(modelPath string, inputNames, outputNames []string, inputs, outputs []ort.Value) (*ort.AdvancedSession, error) {
	opts := ort.NewCUDASessionOptions()
	sess, err := ort.NewAdvancedSession(modelPath, inputNames, outputNames, inputs, outputs, opts)
	if opts != nil {
		opts.Destroy()
	}
	return sess, err
}

func (e *Engine) init() error {
	if err := ort.SetSharedLibraryPath(e.libPath); err != nil {
		return fmt.Errorf("set library path: %w", err)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("init ort: %w", err)
	}

	if err := e.initVisionEncoder(); err != nil {
		e.cleanup()
		return err
	}
	if err := e.initEmbedTokens(); err != nil {
		e.cleanup()
		return err
	}
	if err := e.initEncoderModel(); err != nil {
		e.cleanup()
		return err
	}
	if err := e.initDecoderModel(); err != nil {
		e.cleanup()
		return err
	}

	e.initialized = true
	return nil
}

// ── 1. vision_encoder: pixel_values → image_features ──

func (e *Engine) initVisionEncoder() error {
	path := modelPath(e.modelsDir, e.variant, "vision_encoder.onnx")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("vision_encoder model not found: %w\nRun 'aigc-cli vision init' first", err)
	}

	shape := ort.NewShape(1, Channels, InputSize, InputSize)
	pixels := make([]float32, 1*Channels*InputSize*InputSize)
	var err error
	e.visionPixels, err = ort.NewTensor(shape, pixels)
	if err != nil {
		return fmt.Errorf("create vision pixels tensor: %w", err)
	}

	outShape := ort.NewShape(1, ImageSeqLength, HiddenDim)
	outData := make([]float32, 1*ImageSeqLength*HiddenDim)
	e.visionOutput, err = ort.NewTensor(outShape, outData)
	if err != nil {
		e.visionPixels.Destroy()
		return fmt.Errorf("create vision output tensor: %w", err)
	}

	e.visionSession, err = e.newSession(
		path,
		[]string{"pixel_values"},
		[]string{"image_features"},
		[]ort.Value{e.visionPixels},
		[]ort.Value{e.visionOutput},
	)
	if err != nil {
		return fmt.Errorf("create vision_encoder session: %w", err)
	}
	return nil
}

// ── 2. embed_tokens: input_ids → inputs_embeds ──

func (e *Engine) initEmbedTokens() error {
	path := modelPath(e.modelsDir, e.variant, "embed_tokens.onnx")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("embed_tokens model not found: %w", err)
	}

	inShape := ort.NewShape(1, 1)
	var err error
	e.embedInput, err = ort.NewTensor(inShape, []int64{0})
	if err != nil {
		return fmt.Errorf("create embed input tensor: %w", err)
	}

	outShape := ort.NewShape(1, 1, HiddenDim)
	outData := make([]float32, HiddenDim)
	e.embedOutput, err = ort.NewTensor(outShape, outData)
	if err != nil {
		e.embedInput.Destroy()
		return fmt.Errorf("create embed output tensor: %w", err)
	}

	e.embedSession, err = e.newSession(
		path,
		[]string{"input_ids"},
		[]string{"inputs_embeds"},
		[]ort.Value{e.embedInput},
		[]ort.Value{e.embedOutput},
	)
	if err != nil {
		return fmt.Errorf("create embed_tokens session: %w", err)
	}
	return nil
}

// ── 3. encoder_model: attention_mask + inputs_embeds → last_hidden_state ──

func (e *Engine) initEncoderModel() error {
	path := modelPath(e.modelsDir, e.variant, "encoder_model.onnx")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("encoder_model not found: %w", err)
	}

	maxSeq := 1024 // model's max_position_embeddings
	inShape := ort.NewShape(1, int64(maxSeq), HiddenDim)
	inData := make([]float32, maxSeq*HiddenDim)
	var err error
	e.encInputEmbeds, err = ort.NewTensor(inShape, inData)
	if err != nil {
		return fmt.Errorf("create encoder embeds tensor: %w", err)
	}

	maskShape := ort.NewShape(1, int64(maxSeq))
	maskData := make([]int64, maxSeq)
	for i := range maskData {
		maskData[i] = 1
	}
	e.encAttentionMask, err = ort.NewTensor(maskShape, maskData)
	if err != nil {
		e.encInputEmbeds.Destroy()
		return fmt.Errorf("create encoder mask tensor: %w", err)
	}

	outShape := ort.NewShape(1, int64(maxSeq), HiddenDim)
	outData := make([]float32, maxSeq*HiddenDim)
	e.encOutput, err = ort.NewTensor(outShape, outData)
	if err != nil {
		e.encInputEmbeds.Destroy()
		e.encAttentionMask.Destroy()
		return fmt.Errorf("create encoder output tensor: %w", err)
	}

	e.encoderSession, err = e.newSession(
		path,
		[]string{"attention_mask", "inputs_embeds"},
		[]string{"last_hidden_state"},
		[]ort.Value{e.encAttentionMask, e.encInputEmbeds},
		[]ort.Value{e.encOutput},
	)
	if err != nil {
		return fmt.Errorf("create encoder_model session: %w", err)
	}
	return nil
}

// ── 4. decoder_model: autoregressive text decoder ──

func (e *Engine) initDecoderModel() error {
	path := modelPath(e.modelsDir, e.variant, "decoder_model.onnx")
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("decoder_model not found: %w", err)
	}

	var err error

	maxDecLen := 256
	decEmbShape := ort.NewShape(1, int64(maxDecLen), HiddenDim)
	decEmbData := make([]float32, maxDecLen*HiddenDim)
	e.decInputEmbeds, err = ort.NewTensor(decEmbShape, decEmbData)
	if err != nil {
		return fmt.Errorf("create decoder embeds tensor: %w", err)
	}

	// encoder_hidden_states: [1, 577, 768] (image features only)
	encStateShape := ort.NewShape(1, ImageSeqLength, HiddenDim)
	encStateData := make([]float32, 1*ImageSeqLength*HiddenDim)
	e.decEncStates, err = ort.NewTensor(encStateShape, encStateData)
	if err != nil {
		e.decInputEmbeds.Destroy()
		return fmt.Errorf("create decoder enc_states tensor: %w", err)
	}

	// encoder_attention_mask: [1, 577] (all ones)
	maskShape := ort.NewShape(1, ImageSeqLength)
	maskData := make([]int64, ImageSeqLength)
	for i := range maskData {
		maskData[i] = 1
	}
	e.decEncAttnMask, err = ort.NewTensor(maskShape, maskData)
	if err != nil {
		e.decInputEmbeds.Destroy()
		e.decEncStates.Destroy()
		return fmt.Errorf("create decoder mask tensor: %w", err)
	}

	outShape := ort.NewShape(1, int64(maxDecLen), VocabSize)
	outData := make([]float32, maxDecLen*VocabSize)
	e.decOutput, err = ort.NewTensor(outShape, outData)
	if err != nil {
		return fmt.Errorf("create decoder output tensor: %w", err)
	}

	// Decoder session (fixed-size, created once)
	decoderInputNames := []string{"inputs_embeds", "encoder_hidden_states", "encoder_attention_mask"}
	decoderInputs := []ort.Value{e.decInputEmbeds, e.decEncStates, e.decEncAttnMask}
	decoderOutputNames := []string{"logits"}
	decoderOutputs := []ort.Value{e.decOutput}

	e.decoderSession, err = e.newSession(
		path,
		decoderInputNames,
		decoderOutputNames,
		decoderInputs,
		decoderOutputs,
	)
	if err != nil {
		return fmt.Errorf("create decoder_model session: %w", err)
	}
	return nil
}

// ── Public API ──

func (e *Engine) Describe(path string) (string, error) {
	return e.generate(path, ExpandPrompt(string(TaskDetailedCaption)))
}

// generate runs the full inference pipeline.
//
// Flow:
//  1. Preprocess image → pixel_values tensor
//  2. Tokenize prompt → input_ids
//  3. vision_encoder(pixel_values) → image_features [1, 577, 768]
//  4. embed_tokens(input_ids) → text_embeds [1, prompt_len, 768]
//  5. Concat [image_features; text_embeds] → full_embeds
//  6. encoder_model(attention_mask, full_embeds) → last_hidden_state
//  7. Extract encoder_hidden_states (image part) for decoder cross-attn
//  8. Autoregressive decoder loop
//  9. Detokenize output IDs → text
func (e *Engine) generate(path, prompt string) (string, error) {
	if !e.initialized {
		return "", fmt.Errorf("engine not initialized")
	}

	// 1. Preprocess image (768×768)
	pixels, err := PreprocessImage(path)
	if err != nil {
		return "", fmt.Errorf("preprocess image: %w", err)
	}

	// 2. Tokenize prompt
	inputIDs := e.tokenizer.Encode(prompt)
	if len(inputIDs) == 0 {
		return "", fmt.Errorf("prompt tokenization produced empty output")
	}

	// 3. Run vision encoder → image_features
	imageFeatures, err := e.runVisionEncoder(pixels)
	if err != nil {
		return "", fmt.Errorf("vision encoder: %w", err)
	}

	// 4. Run embed_tokens on prompt → text_embeds
	textEmbeds, err := e.runEmbedTokens(inputIDs)
	if err != nil {
		return "", fmt.Errorf("embed tokens: %w", err)
	}

	// 5. Concat: [image_features; text_embeds]
	fullEmbeds := append(imageFeatures, textEmbeds...)

	// 6. Run encoder_model
	totalSeqLen := ImageSeqLength + len(inputIDs)
	encStates, err := e.runEncoderModel(fullEmbeds, totalSeqLen)
	if err != nil {
		return "", fmt.Errorf("encoder model: %w", err)
	}

	// 7. Extract first ImageSeqLength tokens for decoder cross-attention
	encStatesImage := make([]float32, ImageSeqLength*HiddenDim)
	imagePartSize := ImageSeqLength * HiddenDim
	if len(encStates) >= imagePartSize {
		copy(encStatesImage, encStates[:imagePartSize])
	} else {
		copy(encStatesImage, encStates)
	}

	// 8. Autoregressive decoder loop
	var generatedIDs []int64
	const startTokenID = 2

	for i := 0; i < MaxTokens; i++ {
		decoderIDs := make([]int64, 0, 1+len(generatedIDs))
		decoderIDs = append(decoderIDs, startTokenID)
		decoderIDs = append(decoderIDs, generatedIDs...)

		decEmbeds, err := e.runEmbedTokens(decoderIDs)
		if err != nil {
			return "", fmt.Errorf("embed decoder step %d: %w", i, err)
		}

		logits, err := e.runDecoderStep(decEmbeds, encStatesImage, len(decoderIDs))
		if err != nil {
			return "", fmt.Errorf("decoder step %d: %w", i, err)
		}

		nextID := e.sampler.Sample(logits)
		if nextID == EOSTokenID {
			break
		}
		generatedIDs = append(generatedIDs, nextID)
	}

	// 9. Detokenize & strip special tokens
	result := e.tokenizer.Decode(generatedIDs)
	result = strings.TrimPrefix(result, "<s>")
	result = strings.TrimPrefix(result, "</s>")
	result = strings.TrimSpace(result)
	return result, nil
}

// ── Step runners ──

func (e *Engine) runVisionEncoder(pixels []float32) ([]float32, error) {
	data := e.visionPixels.GetData()
	if len(data) != len(pixels) {
		return nil, fmt.Errorf("vision pixel size mismatch: got %d, need %d", len(data), len(pixels))
	}
	copy(data, pixels)

	if err := e.visionSession.Run(); err != nil {
		return nil, fmt.Errorf("vision_encoder run: %w", err)
	}

	out := e.visionOutput.GetData()
	result := make([]float32, len(out))
	copy(result, out)
	return result, nil
}

func (e *Engine) runEmbedTokens(ids []int64) ([]float32, error) {
	// Since pure-onnx requires fixed-shape tensors, we recreate the embed
	// session each time with the correct input size.
	e.embedSession.Destroy()
	e.embedInput.Destroy()
	e.embedOutput.Destroy()

	n := int64(len(ids))
	inShape := ort.NewShape(1, n)
	var err error
	e.embedInput, err = ort.NewTensor(inShape, ids)
	if err != nil {
		return nil, fmt.Errorf("create embed input: %w", err)
	}
	outShape := ort.NewShape(1, n, HiddenDim)
	outData := make([]float32, int(n)*HiddenDim)
	e.embedOutput, err = ort.NewTensor(outShape, outData)
	if err != nil {
		e.embedInput.Destroy()
		return nil, fmt.Errorf("create embed output: %w", err)
	}
	e.embedSession, err = e.newSession(
		modelPath(e.modelsDir, e.variant, "embed_tokens.onnx"),
		[]string{"input_ids"},
		[]string{"inputs_embeds"},
		[]ort.Value{e.embedInput},
		[]ort.Value{e.embedOutput},
	)
	if err != nil {
		return nil, fmt.Errorf("rebind embed session: %w", err)
	}
	if err := e.embedSession.Run(); err != nil {
		return nil, fmt.Errorf("embed run: %w", err)
	}
	out := e.embedOutput.GetData()
	r := make([]float32, len(out))
	copy(r, out)
	return r, nil
}

func (e *Engine) runEncoderModel(embeds []float32, seqLen int) ([]float32, error) {
	// Update inputs_embeds tensor
	embData := e.encInputEmbeds.GetData()
	if len(embData) < len(embeds) {
		return nil, fmt.Errorf("encoder pre-alloc too small: %d < %d", len(embData), len(embeds))
	}
	copy(embData, embeds)

	// Set attention mask: active positions = 1, padding = 0
	maskData := e.encAttentionMask.GetData()
	for i := 0; i < len(maskData); i++ {
		if i < seqLen {
			maskData[i] = 1
		} else {
			maskData[i] = 0
		}
	}

	if err := e.encoderSession.Run(); err != nil {
		return nil, fmt.Errorf("encoder_model run: %w", err)
	}

	out := e.encOutput.GetData()
	result := make([]float32, len(out))
	copy(result, out)
	return result, nil
}

func (e *Engine) runDecoderStep(tokenEmbeds []float32, encStates []float32, seqLen int) ([]float32, error) {
	// Copy token embeddings into pre-allocated tensor (first seqLen positions)
	embData := e.decInputEmbeds.GetData()
	if len(embData) < seqLen*HiddenDim {
		return nil, fmt.Errorf("decoder pre-alloc too small: %d < %d", len(embData), seqLen*HiddenDim)
	}
	copy(embData[:seqLen*HiddenDim], tokenEmbeds)

	// Copy encoder hidden states
	encData := e.decEncStates.GetData()
	if len(encData) != len(encStates) {
		return nil, fmt.Errorf("decoder enc states mismatch: %d vs %d", len(encData), len(encStates))
	}
	copy(encData, encStates)

	if err := e.decoderSession.Run(); err != nil {
		return nil, fmt.Errorf("decoder run: %w", err)
	}

	out := e.decOutput.GetData()
	r := make([]float32, VocabSize)
	offset := (seqLen - 1) * VocabSize
	if offset >= 0 && offset+VocabSize <= len(out) {
		copy(r, out[offset:offset+VocabSize])
	}
	return r, nil
}

// ── Cleanup ──

func (e *Engine) Close() {
	if e.decOutput != nil {
		e.decOutput.Destroy()
	}
	if e.decEncAttnMask != nil {
		e.decEncAttnMask.Destroy()
	}
	if e.decEncStates != nil {
		e.decEncStates.Destroy()
	}
	if e.decInputEmbeds != nil {
		e.decInputEmbeds.Destroy()
	}
	if e.encoderSession != nil {
		e.encoderSession.Destroy()
	}
	if e.encOutput != nil {
		e.encOutput.Destroy()
	}
	if e.encAttentionMask != nil {
		e.encAttentionMask.Destroy()
	}
	if e.encInputEmbeds != nil {
		e.encInputEmbeds.Destroy()
	}
	if e.embedSession != nil {
		e.embedSession.Destroy()
	}
	if e.embedOutput != nil {
		e.embedOutput.Destroy()
	}
	if e.embedInput != nil {
		e.embedInput.Destroy()
	}
	if e.visionSession != nil {
		e.visionSession.Destroy()
	}
	if e.visionOutput != nil {
		e.visionOutput.Destroy()
	}
	if e.visionPixels != nil {
		e.visionPixels.Destroy()
	}
	ort.DestroyEnvironment()
}

func (e *Engine) cleanup() {
	if e.decOutput != nil {
		e.decOutput.Destroy()
	}
	if e.decEncAttnMask != nil {
		e.decEncAttnMask.Destroy()
	}
	if e.decEncStates != nil {
		e.decEncStates.Destroy()
	}
	if e.decInputEmbeds != nil {
		e.decInputEmbeds.Destroy()
	}
	if e.encOutput != nil {
		e.encOutput.Destroy()
	}
	if e.encAttentionMask != nil {
		e.encAttentionMask.Destroy()
	}
	if e.encInputEmbeds != nil {
		e.encInputEmbeds.Destroy()
	}
	if e.embedOutput != nil {
		e.embedOutput.Destroy()
	}
	if e.embedInput != nil {
		e.embedInput.Destroy()
	}
	if e.visionOutput != nil {
		e.visionOutput.Destroy()
	}
	if e.visionPixels != nil {
		e.visionPixels.Destroy()
	}
	ort.DestroyEnvironment()
}

// IsReady checks if all model files exist.
func IsReady(modelsDir, variant string) bool {
	files := []string{
		"vision_encoder.onnx",
		"embed_tokens.onnx",
		"encoder_model.onnx",
		"decoder_model.onnx",
	}
	for _, f := range files {
		if _, err := os.Stat(modelPath(modelsDir, variant, f)); err != nil {
			return false
		}
	}
	return true
}
