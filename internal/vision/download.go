package vision

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/service"
)

const modelsBaseURL = "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"

// variantInfo maps model variant ID to file prefix.
type variantInfo struct {
	prefix string // filename prefix on the release
	desc   string
	size   string
}

var variantInfos = map[string]variantInfo{
	"base-int8": {
		prefix: "vision_base-int8",
		desc:   "Florence-2 Base INT8 (quantized)",
		size:   "~270 MB",
	},
}

// ModelFile describes a downloadable model file.
type ModelFile struct {
	URL      string
	Filename string
	Size     string
}

// ModelFiles returns the list of files to download for a given variant.
func ModelFiles(variant string) ([]ModelFile, error) {
	info, ok := variantInfos[variant]
	if !ok {
		return nil, fmt.Errorf("unknown variant %q", variant)
	}

	base := fmt.Sprintf("%s/%s", modelsBaseURL, info.prefix)
	return []ModelFile{
		{
			URL:      fmt.Sprintf("%s_vision_encoder.onnx", base),
			Filename: "vision_encoder.onnx",
			Size:     "~94 MB",
		},
		{
			URL:      fmt.Sprintf("%s_encoder_model.onnx", base),
			Filename: "encoder_model.onnx",
			Size:     "~44 MB",
		},
		{
			URL:      fmt.Sprintf("%s_decoder_model.onnx", base),
			Filename: "decoder_model.onnx",
			Size:     "~98 MB",
		},
		{
			URL:      fmt.Sprintf("%s_embed_tokens.onnx", base),
			Filename: "embed_tokens.onnx",
			Size:     "~39 MB",
		},
		{
			URL:      fmt.Sprintf("%s_vocab.json", base),
			Filename: "vocab.json",
			Size:     "~1 MB",
		},
		{
			URL:      fmt.Sprintf("%s_merges.txt", base),
			Filename: "merges.txt",
			Size:     "~0.5 MB",
		},
	}, nil
}

// InitModel downloads all model files for the given variant.
func InitModel(modelsDir, variant string, force bool) error {
	if _, ok := variantInfos[variant]; !ok {
		return fmt.Errorf("unknown variant %q", variant)
	}

	resolvedDir := VariantDir(modelsDir, variant)
	if err := os.MkdirAll(resolvedDir, 0755); err != nil {
		return fmt.Errorf("create model directory: %w", err)
	}

	files, err := ModelFiles(variant)
	if err != nil {
		return err
	}

	for _, f := range files {
		targetPath := filepath.Join(resolvedDir, f.Filename)
		if !force {
			if _, err := os.Stat(targetPath); err == nil {
				fmt.Printf("  Already exists: %s\n", f.Filename)
				continue
			}
		}
		fmt.Printf("  Downloading %s (%s)...\n", f.Filename, f.Size)
		if err := service.SaveResource(f.URL, targetPath); err != nil {
			return fmt.Errorf("download %s: %w", f.Filename, err)
		}
		fmt.Printf("    Saved: %s\n", targetPath)
	}
	return nil
}

// ListModels prints available model variants to stdout.
func ListModels() {
	fmt.Println("Available vision models:")
	for _, v := range AvailableModelVariants {
		marker := ""
		if v.ID == DefaultModelVariant {
			marker = " (default)"
		}
		fmt.Printf("  %-15s %-25s %s%s\n", v.ID, v.Description, v.Size, marker)
	}
}
