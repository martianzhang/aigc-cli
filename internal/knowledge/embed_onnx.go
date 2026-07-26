//go:build cgo

package knowledge

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	ort "github.com/amikos-tech/pure-onnx/ort"
)

type ONNXEmbedder struct {
	dim        int
	session    *ort.AdvancedSession
	tokenizer  *Tokenizer
	inputIDs   *ort.Tensor[int64]
	attention  *ort.Tensor[int64]
	tokenTypes *ort.Tensor[int64]
	output     *ort.Tensor[float32]
}

func NewONNXEmbedder(modelDir, libPath string) (*ONNXEmbedder, error) {
	modelPath := filepath.Join(modelDir, "model.onnx")
	tokPath := filepath.Join(modelDir, "tokenizer.json")

	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("model not found: %w", err)
	}
	if _, err := os.Stat(tokPath); err != nil {
		return nil, fmt.Errorf("tokenizer not found: %w", err)
	}

	tokData, err := os.ReadFile(tokPath)
	if err != nil {
		return nil, fmt.Errorf("read tokenizer: %w", err)
	}
	tokenizer, err := NewTokenizer(tokData)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}

	if err := ort.SetSharedLibraryPath(libPath); err != nil {
		return nil, fmt.Errorf("set library path: %w", err)
	}
	if err := ort.InitializeEnvironment(); err != nil {
		return nil, fmt.Errorf("init environment: %w", err)
	}

	seqLen := int64(128)
	dim := int64(384)

	makeTensor := func() (*ort.Tensor[int64], error) {
		shape := ort.NewShape(1, seqLen)
		data := make([]int64, seqLen)
		return ort.NewTensor(shape, data)
	}

	inputTensor, err := makeTensor()
	if err != nil {
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create input_ids: %w", err)
	}
	attnTensor, err := makeTensor()
	if err != nil {
		inputTensor.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create attention_mask: %w", err)
	}
	tokTypeTensor, err := makeTensor()
	if err != nil {
		inputTensor.Destroy()
		attnTensor.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create token_type_ids: %w", err)
	}

	outShape := ort.NewShape(1, seqLen, dim)
	outData := make([]float32, seqLen*dim)
	outputTensor, err := ort.NewTensor(outShape, outData)
	if err != nil {
		inputTensor.Destroy()
		attnTensor.Destroy()
		tokTypeTensor.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create output: %w", err)
	}

	opts := ort.NewCUDASessionOptions()
	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.Value{inputTensor, attnTensor, tokTypeTensor},
		[]ort.Value{outputTensor},
		opts,
	)
	if opts != nil {
		opts.Destroy()
	}
	if err != nil {
		inputTensor.Destroy()
		attnTensor.Destroy()
		tokTypeTensor.Destroy()
		outputTensor.Destroy()
		ort.DestroyEnvironment()
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &ONNXEmbedder{
		dim: int(dim), session: session, tokenizer: tokenizer,
		inputIDs: inputTensor, attention: attnTensor,
		tokenTypes: tokTypeTensor, output: outputTensor,
	}, nil
}

func (e *ONNXEmbedder) Embed(text string) (Embedding, error) {
	ids := e.tokenizer.Encode(text)
	seqLen := len(e.inputIDs.GetData())
	if len(ids) > seqLen {
		ids = ids[:seqLen]
	}
	in := e.inputIDs.GetData()
	attn := e.attention.GetData()
	ttype := e.tokenTypes.GetData()
	pad := int64(e.tokenizer.padID)
	for i := 0; i < seqLen; i++ {
		if i < len(ids) {
			in[i] = ids[i]
			attn[i] = 1
		} else {
			in[i] = pad
			attn[i] = 0
		}
		ttype[i] = 0
	}
	if err := e.session.Run(); err != nil {
		return Embedding{}, fmt.Errorf("inference: %w", err)
	}
	dim := e.dim
	out := e.output.GetData()
	actualLen := 0
	for i := 0; i < seqLen; i++ {
		if attn[i] != 0 {
			actualLen++
		}
	}
	if actualLen == 0 {
		return Embedding{}, fmt.Errorf("no tokens")
	}
	pooled := make([]float64, dim)
	for t := 0; t < seqLen; t++ {
		if attn[t] == 0 {
			continue
		}
		off := t * dim
		for d := 0; d < dim; d++ {
			pooled[d] += float64(out[off+d])
		}
	}
	for d := 0; d < dim; d++ {
		pooled[d] /= float64(actualLen)
	}
	var norm float64
	for _, v := range pooled {
		norm += v * v
	}
	if norm > 0 {
		norm = math.Sqrt(norm)
		for d := range pooled {
			pooled[d] /= norm
		}
	}
	var emb Embedding
	for d := 0; d < dim && d < len(emb); d++ {
		emb[d] = float32(pooled[d])
	}
	return emb, nil
}

func (e *ONNXEmbedder) Dim() int { return e.dim }

func (e *ONNXEmbedder) Close() {
	if e.session != nil {
		e.session.Destroy()
	}
	if e.output != nil {
		e.output.Destroy()
	}
	if e.inputIDs != nil {
		e.inputIDs.Destroy()
	}
	if e.attention != nil {
		e.attention.Destroy()
	}
	if e.tokenTypes != nil {
		e.tokenTypes.Destroy()
	}
	ort.DestroyEnvironment()
}
