package upscale

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/service"
	up "github.com/martianzhang/aigc-cli/internal/upscale"
)

func newInitCommand(deps func() Deps) *cobra.Command {
	var (
		models   []string
		list     bool
		listInst bool
		force    bool
	)
	cmd := &cobra.Command{
		Use:          "init",
		Short:        "Download ONNX Runtime and super-resolution models",
		SilenceUsage: true,
		Long: `Download the ONNX Runtime shared library and one or more super-resolution models.

Models are saved to <models_dir>/upscale/. The ONNX Runtime is shared with the
other local commands. Proxy settings are respected. Each model keeps its upstream
license (Real-ESRGAN BSD-3-Clause, Real-CUGAN MIT, Swin2SR Apache-2.0).

Which model? (run 'aigc-cli upscale init --list' for the full list and use cases)
  General photos:        realesr-general-x4v3 (default, fast) | real-esrgan-x4plus (higher quality, slow)
  Anime / illustration:  real-cugan-2x (2x) | real-esrgan-x4plus-anime-6b | real-esrgan-x4plus-anime-4b32f (fast) | real-esrgan-animevideov3 (fastest)
  Lightweight 2x:        swin2sr-lightweight-x2
  Noisy / compressed:    swin2sr-realworld-x4 | swin2sr-compressed-x4
  Clean / classical:     swin2sr-classical-x4`,
		Example: `  aigc-cli upscale init                          # ONNX Runtime + default model
  aigc-cli upscale init --list                   # list models and licenses
  aigc-cli upscale init --list-installed         # show what is already installed
  aigc-cli upscale init --model real-cugan-2x --model swin2sr-lightweight-x2
  aigc-cli upscale init --force                  # re-download`,
		RunE: func(cmd *cobra.Command, args []string) error {
			d := deps()
			if list {
				printModelList()
				return nil
			}
			if listInst {
				printInstalled(d.ModelsDir)
				return nil
			}
			return runInit(d, models, force)
		},
	}
	cmd.Flags().StringSliceVar(&models, "model", nil, "model id(s) to download (repeatable)")
	cmd.Flags().BoolVar(&list, "list", false, "list available models and licenses")
	cmd.Flags().BoolVar(&listInst, "list-installed", false, "list installed models")
	cmd.Flags().BoolVar(&force, "force", false, "re-download even if files already exist")
	return cmd
}

func runInit(d Deps, models []string, force bool) error {
	sharedDir := d.ModelsDir
	if err := os.MkdirAll(sharedDir, 0755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}
	if _, err := onnxrt.EnsureInstalled(sharedDir, force); err != nil {
		return err
	}
	onnxrt.EnsureGPUInstalled(sharedDir, force)

	if len(models) == 0 {
		models = []string{up.DefaultModelID}
		fmt.Printf("No model specified; downloading default: %s\n", up.DefaultModelID)
	}

	dir := up.Dir(d.ModelsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create upscale dir: %w", err)
	}

	for _, id := range models {
		info, err := up.Lookup(id)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, info.File)
		if _, err := os.Stat(target); err == nil && !force {
			fmt.Printf("%s already installed: %s\n", info.ID, target)
			continue
		}
		fmt.Printf("Downloading %s (%s, ~%.1fMB, %s)...\n", info.Name, info.ID, info.SizeMB, info.License)
		if err := service.SaveResource(up.URL(info), target); err != nil {
			return fmt.Errorf("download %s: %w", info.ID, err)
		}
		fmt.Printf("  Saved: %s\n", target)
	}
	fmt.Println("Done.")
	return nil
}

func printModelList() {
	fmt.Printf("Available upscale models (saved to %s):\n\n", up.Dir("<models_dir>"))
	for _, m := range up.Models {
		marker := "  "
		if m.ID == up.DefaultModelID {
			marker = "* "
		}
		fmt.Printf("  %s%-30s  x%d  %6.1fMB  %-14s %s\n", marker, m.ID, m.Scale, m.SizeMB, m.License, m.Name)
		fmt.Printf("      %s\n", m.Use)
	}
	fmt.Println("\n  * = default.")
	fmt.Println("  Licenses (upstream weights): Real-ESRGAN BSD-3-Clause, Real-CUGAN MIT, Swin2SR Apache-2.0.")
	fmt.Println("  Quick pick: general → realesr-general-x4v3; photo HQ → real-esrgan-x4plus;")
	fmt.Println("              anime → real-cugan-2x / real-esrgan-x4plus-anime-6b / anime-4b32f (faster);")
	fmt.Println("              fastest anime/video → real-esrgan-animevideov3;")
	fmt.Println("              noisy/compressed → swin2sr-realworld-x4 | swin2sr-compressed-x4; classical → swin2sr-classical-x4.")
}

func printInstalled(modelsDir string) {
	dir := up.Dir(modelsDir)
	var found bool
	for _, m := range up.Models {
		if _, err := os.Stat(filepath.Join(dir, m.File)); err == nil {
			fmt.Printf("  %-30s %s\n", m.ID, filepath.Join(dir, m.File))
			found = true
		}
	}
	if !found {
		fmt.Printf("No upscale models installed in %s\n  Run 'aigc-cli upscale init' to download one.\n", dir)
	}
}
