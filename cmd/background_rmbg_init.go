package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	rmbgModelID       = "rmbg-2.0"
	rmbgModelFilename = "model-rmbg-2.0.onnx"

	// Default download: community mirror (camenduru), no auth required.
	// Same weights as the official briaai/RMBG-2.0, same license.
	// model_quantized.onnx is the best balance of quality, size, and ORT compatibility.
	rmbgDefaultURL = "https://huggingface.co/camenduru/RMBG-2.0/resolve/main/onnx/model_quantized.onnx"

	// Official URL: briaai/RMBG-2.0 is gated — requires HF_TOKEN.
	rmbgOfficialURL = "https://huggingface.co/briaai/RMBG-2.0/resolve/main/onnx/model_quantized.onnx"

	rmbgModelDesc = "RMBG 2.0 (BiRefNet), 176M params, INT8 quantized"
	rmbgModelSize = "366MB"
)

// backgroundInitCmd 表示 `aigc-cli background init` 子命令。
var backgroundInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download ONNX Runtime and RMBG 2.0 model",
	SilenceUsage: true,
	Long: `Download the ONNX Runtime shared library and the RMBG 2.0 model.

The runtime and model are saved to ~/.config/aigc-cli/models/ for offline
AI background removal via the 'background' command.

The ONNX Runtime is shared with the 'detect' command — if you already
ran 'aigc-cli detect init', only the RMBG model will be downloaded.

Model source:
  Default: camenduru/RMBG-2.0 (community mirror, ~366MB INT8 quantized, no auth required)
  With --hf-token: official briaai/RMBG-2.0 (gated, requires HuggingFace token)

Use --force to re-download existing files.

Note: The RMBG 2.0 model weights are CC BY-NC 4.0 (non-commercial).
For commercial use, contact BRIA AI for a license.

Proxy settings from config.yaml, env vars (HTTP_PROXY), or --http-proxy
flag are automatically respected.`,
	RunE: runBackgroundInit,
}

var (
	bgInitForce bool
	bgHFToken   string
)

func runBackgroundInit(cmd *cobra.Command, args []string) error {
	modelsDir := rmbgModelsDir()
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return fmt.Errorf("cannot create directory %s: %w", modelsDir, err)
	}

	// ── Download ONNX Runtime CPU (shared with detect) ──
	if err := downloadORT(modelsDir, getORTDownloadInfo(), bgInitForce); err != nil {
		return err
	}
	// ── Download ONNX Runtime GPU (where available) ──
	if gpu := getGPUORTDownloadInfo(); gpu != nil {
		if err := downloadORT(modelsDir, *gpu, bgInitForce); err != nil {
			return err
		}
	}

	// ── Download RMBG 2.0 model ──
	modelPath := filepath.Join(modelsDir, rmbgModelFilename)
	if _, err := os.Stat(modelPath); err == nil && !bgInitForce {
		fmt.Printf("RMBG model already exists: %s\n  Use --force to re-download.\n", modelPath)
		return nil
	}

	// Determine URL: --hf-token → official gated repo, else → community mirror
	modelURL := rmbgDefaultURL
	if bgHFToken != "" {
		modelURL = rmbgOfficialURL
	}

	fmt.Printf("Downloading %s (%s)...\n", rmbgModelDesc, rmbgModelSize)
	fmt.Printf("  URL: %s\n", modelURL)
	if bgHFToken != "" {
		fmt.Println("  (authenticated with --hf-token)")
	}
	fmt.Println("  This may take a while depending on your connection speed.")

	if err := downloadModel(modelURL, modelPath, bgHFToken); err != nil {
		return fmt.Errorf("model download failed: %w", err)
	}
	fmt.Println("  Done.")

	fmt.Println("\nRMBG 2.0 model installed! Run 'aigc-cli background <file> --remove' to use AI background removal.")
	return nil
}

// downloadModel 下载模型文件，可选 HuggingFace token 认证。
func downloadModel(url, dest, token string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		fmt.Fprintln(os.Stderr, "This model requires HuggingFace authentication. Use --hf-token:")
		fmt.Fprintln(os.Stderr, "  1. Accept terms at https://huggingface.co/briaai/RMBG-2.0")
		fmt.Fprintln(os.Stderr, "  2. Create a token at https://huggingface.co/settings/tokens")
		fmt.Fprintln(os.Stderr, "  3. aigc-cli background init --hf-token hf_...")
		return fmt.Errorf("HTTP 401 unauthorized")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpDest := dest + ".tmp"
	f, err := os.Create(tmpDest)
	if err != nil {
		return fmt.Errorf("cannot create %s: %w", tmpDest, err)
	}
	written, err := io.Copy(f, resp.Body)
	if err != nil {
		f.Close()
		os.Remove(tmpDest)
		return fmt.Errorf("download failed: %w", err)
	}
	f.Close()
	if written == 0 {
		os.Remove(tmpDest)
		return fmt.Errorf("downloaded file is empty")
	}
	if err := os.Rename(tmpDest, dest); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	return nil
}

func init() {
	backgroundCmd.AddCommand(backgroundInitCmd)
	backgroundInitCmd.Flags().BoolVar(&bgInitForce, "force", false, "re-download even if files already exist")
	backgroundInitCmd.Flags().StringVar(&bgHFToken, "hf-token", "", "HuggingFace token for gated model (official briaai/RMBG-2.0)")
}
