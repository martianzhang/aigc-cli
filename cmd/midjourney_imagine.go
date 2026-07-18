package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/types"
)

var mjImagineCmd = &cobra.Command{
	Use:   "imagine",
	Short: "Text-to-image / image-guided generation",
	Long: `Generate images from a text prompt, optionally with reference images.

The default MJ entry point. Supports all MJ structured fields and native flags.

Examples:
  aigc-cli midjourney imagine --prompt "a cute cat --ar 16:9"
  aigc-cli midjourney imagine --prompt "a cat" --size "16:9" --version "6.1" --style raw
  aigc-cli midjourney imagine --prompt "luxury product" --image-url ref.png --iw 1.2
  aigc-cli midjourney imagine --json request.json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req, err := buildMJImagineReq(cmd)
		if err != nil {
			return err
		}

		// Merge config defaults
		if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
			cfg.Defaults.Midjourney.MergeIntoImagine(req)
		}

		// Resolve local images
		if len(req.ImageURLs) > 0 {
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}

		c := newMJClient()
		return runMJSubmitAndPoll(c, "imagine", req)
	},
}

// ============================================================================
// Subcommand: blend
// ============================================================================
var mjBlendCmd = &cobra.Command{
	Use:   "blend",
	Short: "Multi-image blend (2-4 images)",
	Long: `Blend 2-4 images into a new image. No prompt is used — pure image blend.

Examples:
  aigc-cli midjourney blend --image-url a.png --image-url b.png
  aigc-cli midjourney blend --image-url a.png --image-url b.png --image-url c.png --dimensions PORTRAIT`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJBlendRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if len(req.ImageURLs) < 2 {
				return fmt.Errorf("at least 2 image_urls required")
			}
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
			return runMJSubmitAndPoll(c, "blend", req)
		}

		if len(mjImageURLs) < 2 {
			return fmt.Errorf("at least 2 --image-url required for blend")
		}

		req := &types.MJBlendRequest{
			ImageURLs:  mjImageURLs,
			Dimensions: mjDimensions,
			Size:       mjSize,
			Speed:      mjSpeed,
		}

		c := newMJClient()
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
		return runMJSubmitAndPoll(c, "blend", req)
	},
}

// ============================================================================
// Subcommand: describe
// ============================================================================
var mjDescribeCmd = &cobra.Command{
	Use:   "describe",
	Short: "Image to text (reverse prompt)",
	Long: `Reverse-engineer a prompt from an image. Returns 4 prompt suggestions.

Example:
  aigc-cli midjourney describe --image-url input.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if mjJSONInput != "" {
			data, err := readInput(mjJSONInput)
			if err != nil {
				return fmt.Errorf("failed to read JSON input: %w", err)
			}
			req := &types.MJDescribeRequest{}
			if err := json.Unmarshal(data, req); err != nil {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			if len(req.ImageURLs) == 0 {
				return fmt.Errorf("image_urls is required")
			}
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
			return runMJSubmitAndPoll(c, "describe", req)
		}

		if len(mjImageURLs) == 0 {
			return fmt.Errorf("--image-url is required for describe")
		}

		req := &types.MJDescribeRequest{
			ImageURLs: mjImageURLs,
			Speed:     mjSpeed,
		}
		c := newMJClient()
		resolved, err := c.ResolveLocalImages(req.ImageURLs)
		if err != nil {
			return fmt.Errorf("failed to resolve image-urls: %w", err)
		}
		req.ImageURLs = resolved
		return runMJSubmitAndPoll(c, "describe", req)
	},
}

// ============================================================================
// Subcommand: edits
// ============================================================================
var mjEditsCmd = &cobra.Command{
	Use:   "edits",
	Short: "Image edit (rewrite whole image)",
	Long: `Rewrite an entire image from a prompt + reference image.
Good for background replacement, style transfer, and content changes.

Example:
  aigc-cli midjourney edits --prompt "replace background with a modern kitchen" --image-url product.png`,
	RunE: func(cmd *cobra.Command, args []string) error {
		req, err := buildMJImagineReq(cmd) // Same structure as imagine
		if err != nil {
			return err
		}
		if len(req.ImageURLs) == 0 {
			return fmt.Errorf("--image-url is required for edits")
		}

		if cfg, err := config.LoadDefaults(shared.CfgFile); err == nil && cfg != nil && cfg.Defaults != nil {
			cfg.Defaults.Midjourney.MergeIntoImagine(req)
		}

		if len(req.ImageURLs) > 0 {
			c := newMJClient()
			resolved, err := c.ResolveLocalImages(req.ImageURLs)
			if err != nil {
				return fmt.Errorf("failed to resolve image-urls: %w", err)
			}
			req.ImageURLs = resolved
		}

		c := newMJClient()
		return runMJSubmitAndPoll(c, "edits", req)
	},
}
