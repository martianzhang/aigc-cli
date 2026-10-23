package sfx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	ort "github.com/amikos-tech/pure-onnx/ort"
)

// Engine runs the Stable Audio Open Small ONNX bundle.
type Engine struct {
	dir string

	tokenizer *Tokenizer
	embedder  *secondsEmbedder

	textSess *ort.AdvancedSession
	ditSess  *ort.AdvancedSession
	vaeSess  *ort.AdvancedSession

	idsIn   *ort.Tensor[int64]
	maskIn  *ort.Tensor[int64]
	textOut *ort.Tensor[float32]

	xIn      *ort.Tensor[float32]
	tIn      *ort.Tensor[float32]
	condIn   *ort.Tensor[float32]
	globalIn *ort.Tensor[float32]
	vOut     *ort.Tensor[float32]

	latIn    *ort.Tensor[float32]
	audioOut *ort.Tensor[float32]

	mu     sync.Mutex
	closed bool
}

// NewEngine loads the bundle in dir and initializes ONNX Runtime from libPath.
func NewEngine(dir, libPath string) (*Engine, error) {
	if err := checkBundle(dir); err != nil {
		return nil, err
	}
	tok, err := NewTokenizer(filepath.Join(dir, "t5_tokenizer_json.json"))
	if err != nil {
		return nil, err
	}
	emb, err := loadSecondsEmbedder(filepath.Join(dir, "seconds_embedder.json"))
	if err != nil {
		return nil, err
	}

	e := &Engine{dir: dir, tokenizer: tok, embedder: emb}
	if err := e.initRuntime(libPath); err != nil {
		e.Close()
		return nil, err
	}
	return e, nil
}

func checkBundle(dir string) error {
	for _, name := range RequiredFiles {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("model file %s is missing: %w", name, err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("model file %s is empty", name)
		}
	}
	return nil
}

func (e *Engine) initRuntime(libPath string) error {
	if err := ort.SetSharedLibraryPath(libPath); err != nil && !ort.IsInitialized() {
		return fmt.Errorf("set onnxruntime path: %w", err)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("init onnxruntime: %w", err)
	}

	var err error
	if e.idsIn, err = ort.NewEmptyTensor[int64](ort.NewShape(1, TextTokens)); err != nil {
		return fmt.Errorf("create ids tensor: %w", err)
	}
	if e.maskIn, err = ort.NewEmptyTensor[int64](ort.NewShape(1, TextTokens)); err != nil {
		return fmt.Errorf("create mask tensor: %w", err)
	}
	if e.textOut, err = ort.NewEmptyTensor[float32](ort.NewShape(1, TextTokens, TextDim)); err != nil {
		return fmt.Errorf("create text output tensor: %w", err)
	}
	if e.xIn, err = ort.NewEmptyTensor[float32](ort.NewShape(1, LatentDim, LatentFrames)); err != nil {
		return fmt.Errorf("create latent tensor: %w", err)
	}
	if e.tIn, err = ort.NewEmptyTensor[float32](ort.NewShape(1)); err != nil {
		return fmt.Errorf("create timestep tensor: %w", err)
	}
	if e.condIn, err = ort.NewEmptyTensor[float32](ort.NewShape(1, TextTokens+1, TextDim)); err != nil {
		return fmt.Errorf("create conditioning tensor: %w", err)
	}
	if e.globalIn, err = ort.NewEmptyTensor[float32](ort.NewShape(1, TextDim)); err != nil {
		return fmt.Errorf("create global conditioning tensor: %w", err)
	}
	if e.vOut, err = ort.NewEmptyTensor[float32](ort.NewShape(1, LatentDim, LatentFrames)); err != nil {
		return fmt.Errorf("create velocity tensor: %w", err)
	}
	if e.latIn, err = ort.NewEmptyTensor[float32](ort.NewShape(1, LatentDim, LatentFrames)); err != nil {
		return fmt.Errorf("create vae input tensor: %w", err)
	}
	if e.audioOut, err = ort.NewEmptyTensor[float32](ort.NewShape(1, Channels, AudioFrames)); err != nil {
		return fmt.Errorf("create vae output tensor: %w", err)
	}

	if e.textSess, err = newSession(e, "t5_text_encoder.onnx",
		[]string{"input_ids", "attention_mask"}, []string{"embeddings"},
		[]ort.Value{e.idsIn, e.maskIn}, []ort.Value{e.textOut}); err != nil {
		return err
	}
	if e.ditSess, err = newSession(e, "dit_step.onnx",
		[]string{"x", "t", "cross_attn_cond", "global_cond"}, []string{"v"},
		[]ort.Value{e.xIn, e.tIn, e.condIn, e.globalIn}, []ort.Value{e.vOut}); err != nil {
		return err
	}
	if e.vaeSess, err = newSession(e, "vae_decoder.onnx",
		[]string{"latent"}, []string{"audio"},
		[]ort.Value{e.latIn}, []ort.Value{e.audioOut}); err != nil {
		return err
	}
	return nil
}

func newSession(e *Engine, name string, inputNames, outputNames []string, inputs, outputs []ort.Value) (*ort.AdvancedSession, error) {
	path := filepath.Join(e.dir, name)
	opts := ort.NewCUDASessionOptions()
	sess, err := ort.NewAdvancedSession(path, inputNames, outputNames, inputs, outputs, opts)
	if opts != nil {
		opts.Destroy()
	}
	if err != nil {
		return nil, fmt.Errorf("create %s session: %w", name, err)
	}
	return sess, nil
}

// Close releases ONNX sessions and tensors. It is safe to call twice.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true

	var firstErr error
	for _, sess := range []*ort.AdvancedSession{e.textSess, e.ditSess, e.vaeSess} {
		if sess != nil {
			if err := sess.Destroy(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}

	var tensors []interface{ Destroy() error }
	if e.idsIn != nil {
		tensors = append(tensors, e.idsIn)
	}
	if e.maskIn != nil {
		tensors = append(tensors, e.maskIn)
	}
	if e.textOut != nil {
		tensors = append(tensors, e.textOut)
	}
	if e.xIn != nil {
		tensors = append(tensors, e.xIn)
	}
	if e.tIn != nil {
		tensors = append(tensors, e.tIn)
	}
	if e.condIn != nil {
		tensors = append(tensors, e.condIn)
	}
	if e.globalIn != nil {
		tensors = append(tensors, e.globalIn)
	}
	if e.vOut != nil {
		tensors = append(tensors, e.vOut)
	}
	if e.latIn != nil {
		tensors = append(tensors, e.latIn)
	}
	if e.audioOut != nil {
		tensors = append(tensors, e.audioOut)
	}
	for _, t := range tensors {
		if err := t.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := ort.DestroyEnvironment(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}
