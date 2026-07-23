package vision

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/martianzhang/aigc-cli/internal/service"
)

const hfBase = "https://huggingface.co/Heliosoph"

// variantInfo maps model variant ID to HuggingFace repo and file suffix.
type variantInfo struct {
	repo   string // HF repo name
	suffix string // ONNX file suffix (e.g. "quantized", "fp16")
	desc   string
	size   string
}

var variantInfos = map[string]variantInfo{
	"base-int8": {
		repo:   "florence-2-base-ft-quantized-onnx",
		suffix: "quantized",
		desc:   "Florence-2 Base INT8 (quantized)",
		size:   "~270 MB",
	},
	"base-fp16": {
		repo:   "florence-2-base-ft-fp16-onnx",
		suffix: "fp16",
		desc:   "Florence-2 Base FP16",
		size:   "~548 MB",
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

	baseURL := fmt.Sprintf("%s/%s/resolve/main", hfBase, info.repo)

	return []ModelFile{
		{
			URL:      fmt.Sprintf("%s/vision_encoder_%s.onnx", baseURL, info.suffix),
			Filename: "vision_encoder.onnx",
			Size: func() string {
				if variant == "base-fp16" {
					return "~184 MB"
				}
				return "~94 MB"
			}(),
		},
		{
			URL:      fmt.Sprintf("%s/encoder_model_%s.onnx", baseURL, info.suffix),
			Filename: "encoder_model.onnx",
			Size: func() string {
				if variant == "base-fp16" {
					return "~87 MB"
				}
				return "~44 MB"
			}(),
		},
		{
			URL:      fmt.Sprintf("%s/decoder_model_%s.onnx", baseURL, info.suffix),
			Filename: "decoder_model.onnx",
			Size: func() string {
				if variant == "base-fp16" {
					return "~194 MB"
				}
				return "~98 MB"
			}(),
		},
		{
			URL:      fmt.Sprintf("%s/embed_tokens_%s.onnx", baseURL, info.suffix),
			Filename: "embed_tokens.onnx",
			Size: func() string {
				if variant == "base-fp16" {
					return "~79 MB"
				}
				return "~39 MB"
			}(),
		},
		{
			URL:      fmt.Sprintf("%s/vocab.json", baseURL),
			Filename: "vocab.json",
			Size:     "~1 MB",
		},
		{
			URL:      fmt.Sprintf("%s/merges.txt", baseURL),
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
