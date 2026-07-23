// Package vision provides local image understanding via ONNX-based VLM models
// (Florence-2). Supports image captioning (describe) and visual question
// answering (ask) with a shared inference engine.
package vision

import "fmt"

// TaskPrompt is a Florence-2 task token that controls the model's output mode.
type TaskPrompt string

const (
	TaskCaption             TaskPrompt = "<CAPTION>"
	TaskDetailedCaption     TaskPrompt = "<DETAILED_CAPTION>"
	TaskMoreDetailedCaption TaskPrompt = "<MORE_DETAILED_CAPTION>"
	TaskVQA                 TaskPrompt = "<VQA>"
	TaskOD                  TaskPrompt = "<OD>"
)

// Model constants for Florence-2-base-ft (Heliosoph ONNX export).
const (
	// InputSize is the expected image dimension (768×768).
	InputSize = 768
	// Channels is the number of color channels.
	Channels = 3
	// HiddenDim is the hidden dimension size for all sub-models.
	HiddenDim = 768
	// NumDecoderLayers is the number of decoder layers.
	NumDecoderLayers = 6
	// NumAttentionHeads is the number of attention heads.
	NumAttentionHeads = 12
	// HeadDim is the dimension per attention head.
	HeadDim = 64
	// ImageSeqLength is the number of visual tokens output by vision_encoder.
	ImageSeqLength = 577
	// MaxTokens is the maximum number of tokens to generate per inference.
	MaxTokens = 512
	// VocabSize is the Florence-2 vocabulary size.
	VocabSize = 51289
	// DecoderStartTokenID is the decoder start token (also EOS).
	DecoderStartTokenID = 2
	// EOSTokenID is the end-of-sequence token ID.
	EOSTokenID = 2
	// PadTokenID is the padding token ID.
	PadTokenID = 1
)

// ImageNet normalization parameters (used by Florence-2).
var (
	MeanRGB = [3]float32{0.485, 0.456, 0.406}
	StdRGB  = [3]float32{0.229, 0.224, 0.225}
)

// ModelVariant describes a Florence-2 ONNX model variant.
type ModelVariant struct {
	ID          string
	Description string
	Size        string
}

var AvailableModelVariants = []ModelVariant{
	{ID: "base-int8", Description: "Florence-2 Base INT8 (quantized)", Size: "~270 MB"},
	{ID: "base-fp16", Description: "Florence-2 Base FP16", Size: "~548 MB"},
}

func ResolveModelVariant(id string) (ModelVariant, error) {
	for _, v := range AvailableModelVariants {
		if v.ID == id {
			return v, nil
		}
	}
	return ModelVariant{}, fmt.Errorf("unknown model variant %q (available: base-int8, base-fp16)", id)
}

const DefaultModelVariant = "base-int8"

// taskPromptExpansions maps Florence-2 task tokens to their full prompt text.
// Based on preprocessor_config.json task_prompts_without_inputs.
var taskPromptExpansions = map[string]string{
	string(TaskCaption):             "What does the image describe?",
	string(TaskDetailedCaption):     "Describe in detail what is shown in the image.",
	string(TaskMoreDetailedCaption): "Describe with a paragraph what is shown in the image.",
	string(TaskOD):                  "Locate the objects with category name in the image.",
}

// ExpandPrompt expands a Florence-2 task token into its full prompt text.
// Tokens without an expansion (e.g. <VQA>) are returned as-is.
func ExpandPrompt(prompt string) string {
	if expanded, ok := taskPromptExpansions[prompt]; ok {
		return expanded
	}
	return prompt
}
