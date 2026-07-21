package cmd

import (
	"archive/tar"
	"compress/bzip2"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/martianzhang/aigc-cli/internal/audio"
	"github.com/martianzhang/aigc-cli/internal/onnxrt"
	"github.com/martianzhang/aigc-cli/internal/service"
)

var audioInitCmd = &cobra.Command{
	Use:          "init",
	Short:        "Download audio models for local inference",
	SilenceUsage: true,
	Long: `Download ONNX audio models for local TTS and speech recognition.

Models are saved to ~/.config/aigc-cli/models/audio/<model-id>/.

The ONNX Runtime is shared with the 'detect' and 'background' commands.
If not already installed, it will be downloaded automatically.

TTS models:
  kokoro          Multilingual (EN/ZH/JA/KO/FR), 82M params, ~130MB (default)
  kokoro-en       English-only Kokoro, 100MB
  vits-zh-ll      Chinese VITS, 5 speakers, 115MB
  vits-zh-hf-eula Chinese VITS, 804 speakers (natural Chinese), 116MB
  vits-zh-aishell3 Chinese VITS, female, 115MB
  vits-cantonese  Cantonese VITS, 115MB
  vits-ljs        American English VITS (LJSpeech), 115MB
  vits-vctk       British English VITS (VCTK, 109 speakers), 115MB

ASR models:
  whisper-tiny    OpenAI Whisper Tiny, 39M params, ~150MB
  sense-voice     Alibaba SenseVoice, 80M params, ~80MB
  whisper-tiny    OpenAI Whisper Tiny ASR, 39M params, ~150MB
  sense-voice     Alibaba SenseVoice ASR, 80M params, ~80MB

Use --list to see all available models. Use --list-installed to see what
you already have. Proxy settings are automatically respected.`,
	RunE: runAudioInit,
}

var (
	audioInitModel      []string
	audioInitList       bool
	audioInitListInst   bool
	audioInitType       string
	audioInitLang       string
	audioInitForce      bool
	audioInitHFToken    string
	audioInitURL        string
	audioInitName       string
	audioInitListVoices bool
)

func runAudioInit(cmd *cobra.Command, args []string) error {
	modelsDir := audioModelsDir()

	// ── List available models ──
	if audioInitList {
		typ := audio.ModelType(audioInitType)
		if typ != "" && typ != audio.ModelASR && typ != audio.ModelTTS {
			return fmt.Errorf("invalid type %q (choose: asr, tts)", audioInitType)
		}
		models := audio.ListByType(typ, audioInitLang)
		if len(models) == 0 {
			fmt.Println("No models found matching the criteria.")
			return nil
		}
		fmt.Printf("Available models (type=%s lang=%s):\n", audioInitType, audioInitLang)
		for _, m := range models {
			fmt.Printf("  %-16s  %-10s  %-8s  %s\n", m.ID, m.Type, m.Size, m.Description)
		}
		return nil
	}

	// ── List installed models ──
	if audioInitListInst {
		installed, err := audio.ListInstalled(filepath.Dir(audioModelsDir()))
		if err != nil {
			return fmt.Errorf("list installed: %w", err)
		}
		if len(installed) == 0 {
			fmt.Println("No audio models installed. Run 'aigc-cli audio init --model <id>' to download one.")
			return nil
		}
		fmt.Println("Installed audio models:")
		for _, m := range installed {
			fmt.Printf("  %-16s  %-10s  %-8s  %s\n", m.ID, m.Type, m.Size, m.Description)
		}
		return nil
	}

	// ── List voices for a model ──
	if audioInitListVoices {
		if len(audioInitModel) == 0 {
			return fmt.Errorf("specify a model with --model to list its voices")
		}
		modelID := audioInitModel[0]
		modelDir := filepath.Join(audioModelsDir(), modelID)
		if _, err := os.Stat(modelDir); err != nil {
			return fmt.Errorf("model %q not installed, run 'audio init --model %s' first", modelID, modelID)
		}
		engine, err := audio.NewTTSEngine("", modelDir)
		if err != nil {
			return fmt.Errorf("load model: %w", err)
		}
		count := engine.NumSpeakers()
		engine.Close()
		if count <= 0 {
			fmt.Printf("Model %q has 1 voice\n", modelID)
			return nil
		}
		fmt.Printf("Model %q has %d voices (SID 0-%d)\n", modelID, count, count-1)

		names := voiceNamesForModel(modelID, count)
		if len(names) > 0 {
			fmt.Println("\nNamed voices:")
			type vs struct {
				sid  int
				name string
			}
			var sorted []vs
			for sid, name := range names {
				sorted = append(sorted, vs{sid, name})
			}
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].sid < sorted[j].sid })
			for _, v := range sorted {
				fmt.Printf("  %-4d  %s\n", v.sid, v.name)
			}
		} else {
			fmt.Printf("Use --voice <SID> to select a voice (0-%d)\n", count-1)
		}
		return nil
	}

	// ── Download from URL (custom model) ──
	if audioInitURL != "" {
		name := audioInitName
		if name == "" {
			name = "custom-model"
		}
		if err := downloadFromURL(audioInitURL, audioModelsDir(), name, audioInitForce); err != nil {
			return fmt.Errorf("download: %w", err)
		}
		fmt.Printf("Custom model installed as %q.\n", name)
		return nil
	}

	// ── Download models ──
	if len(audioInitModel) == 0 {
		audioInitModel = []string{"kokoro", "sense-voice"}
		fmt.Println("No model specified, downloading defaults: kokoro (TTS) + sense-voice (ASR)")
	}

	// Ensure ONNX Runtime is installed
	if _, err := onnxrt.EnsureInstalled(filepath.Dir(audioModelsDir()), audioInitForce); err != nil {
		return err
	}

	// Ensure sherpa-onnx runtime libraries are available
	if err := ensureAudioRuntime(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: audio runtime not fully installed: %v\n", err)
		fmt.Fprintf(os.Stderr, "  Local TTS/ASR will not work. Use a release build or install GCC and run:\n")
		fmt.Fprintf(os.Stderr, "    bash scripts/build-helper.sh\n")
	}

	for _, modelID := range audioInitModel {
		info, err := audio.Lookup(modelID)
		if err != nil {
			return err
		}
		if err := downloadModelFiles(info, modelsDir, audioInitForce); err != nil {
			return fmt.Errorf("model %q: %w", modelID, err)
		}
		fmt.Printf("Model %q installed. Use 'aigc-cli audio speak --local --input \"...\"' to try it.\n", modelID)
	}
	return nil
}

// downloadModelFiles downloads all files for a model from the registry.
func downloadModelFiles(info audio.ModelInfo, modelsBaseDir string, force bool) error {
	modelDir := filepath.Join(modelsBaseDir, info.ID)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	for _, f := range info.Files {
		dest := filepath.Join(modelDir, f.Path)
		label := info.ID + "/" + f.Path
		if err := downloadSingleFile(f.URL, dest, label, force); err != nil {
			return fmt.Errorf("download %s/%s: %w", info.ID, f.Path, err)
		}
	}
	return nil
}

// downloadFromURL downloads a model from an arbitrary URL (outside registry).
func downloadFromURL(url, baseDir, name string, force bool) error {
	modelDir := filepath.Join(baseDir, name)
	os.MkdirAll(modelDir, 0755)
	filename := filepath.Base(url)
	dest := filepath.Join(modelDir, filename)
	return downloadSingleFile(url, dest, filename, force)
}

// downloadSingleFile downloads a single file, handling tar.bz2 extraction.
func downloadSingleFile(url, dest, label string, force bool) error {
	// For archives, check if the model directory already has content
	if strings.HasSuffix(label, ".tar.bz2") {
		extractedDir := strings.TrimSuffix(dest, ".tar.bz2")
		if fi, err := os.Stat(extractedDir); err == nil && fi.IsDir() {
			// Check if model.onnx exists inside (or any subdirectory)
			entries, _ := os.ReadDir(extractedDir)
			for _, e := range entries {
				if e.IsDir() || strings.HasSuffix(e.Name(), ".onnx") {
					if !force {
						fmt.Printf("%s: already installed (%s)\n", label, url)
						return nil
					}
					break
				}
			}
		}
	} else if _, err := os.Stat(dest); err == nil && !force {
		fmt.Printf("%s: already exists\n", dest)
		return nil
	}
	fmt.Printf("Downloading %s...\n", url)
	if err := service.SaveResource(url, dest); err != nil {
		return fmt.Errorf("download %s: %w", label, err)
	}
	if strings.HasSuffix(label, ".tar.bz2") {
		extractDir := strings.TrimSuffix(dest, ".tar.bz2")
		fmt.Printf("Extracting %s...\n", label)
		if err := extractTarBz2(dest, extractDir); err != nil {
			return fmt.Errorf("extract: %w", err)
		}
		os.Remove(dest)
	}
	return nil
}

// extractTarBz2 extracts a .tar.bz2 archive into the specified directory.
// The top-level directory inside the archive is stripped so files go directly
// into extractDir.
func extractTarBz2(archivePath, extractDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	bz2r := bzip2.NewReader(f)
	tarr := tar.NewReader(bz2r)

	// Detect the top-level directory name to strip it
	var topDir string
	for {
		header, err := tarr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if topDir == "" {
			// First entry: extract the top directory name
			parts := strings.SplitN(header.Name, "/", 2)
			if len(parts) > 1 {
				topDir = parts[0] + "/"
			}
		}

		// Strip the top directory
		relPath := strings.TrimPrefix(header.Name, topDir)
		if relPath == "" {
			continue // skip the top directory entry itself
		}

		target := filepath.Join(extractDir, relPath)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tarr)
			out.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// voiceNamesForModel returns known voice names for a given model, or nil if unknown.
// Currently only kokoro has a complete name mapping. Other models use raw SIDs.
func voiceNamesForModel(modelID string, count int) map[int]string {
	switch modelID {
	case "kokoro", "kokoro-en":
		names := make(map[int]string)
		for name, sid := range audio.KokoroVoiceNames {
			if sid < count {
				names[sid] = name
			}
		}
		return names
	}
	return nil
}

// releaseTag returns the base release tag for downloading runtime assets.
// Dev builds (e.g. "v1.10.2-12-gbcfd94c-dirty") are stripped to the
// underlying release tag ("v1.10.2") so the download URL is valid.
func releaseTag() string {
	tag := Version
	if tag == "" || tag == "dev" {
		return "latest"
	}
	// Strip "-dirty" suffix from git describe output.
	tag = strings.TrimSuffix(tag, "-dirty")
	// Strip git describe suffix: "v1.10.2-12-gbcfd94c" → "v1.10.2".
	if idx := strings.LastIndex(tag, "-g"); idx > 0 {
		prefix := tag[:idx] // "v1.10.2-12"
		if idx2 := strings.LastIndex(prefix, "-"); idx2 > 0 {
			tag = prefix[:idx2] // "v1.10.2"
		} else {
			tag = prefix
		}
	}
	return tag
}

// ensureAudioRuntime ensures the helper library and sherpa-onnx libs are available.
func ensureAudioRuntime() error {
	modelsDir := filepath.Dir(audioModelsDir())

	helperName := map[string]string{
		"darwin":  "libaigc-sherpa-helper.dylib",
		"linux":   "libaigc-sherpa-helper.so",
		"windows": "aigc-sherpa-helper.dll",
	}[runtime.GOOS]
	if helperName == "" {
		return nil
	}
	helperPath := filepath.Join(modelsDir, helperName)

	tag := releaseTag()
	baseURL := fmt.Sprintf("https://github.com/martianzhang/aigc-cli/releases/download/%s", tag)

	if _, err := os.Stat(helperPath); err == nil {
		fmt.Printf("Audio helper: %s\n", helperPath)
	} else if err := downloadAsset(baseURL, helperName, modelsDir); err == nil {
		fmt.Printf("Audio helper: %s\n", helperPath)
	} else if err := compileHelper(helperPath, modelsDir); err != nil {
		return fmt.Errorf("cannot install audio runtime.\nUse a release build or run: bash scripts/build-helper.sh")
	}

	ensureRuntimeLibs(baseURL, modelsDir)
	return nil
}

func ensureRuntimeLibs(baseURL, dir string) {
	libs := map[string][]string{
		"windows": {"sherpa-onnx-c-api.dll", "onnxruntime.dll"},
		"darwin":  {"libsherpa-onnx-c-api.dylib", "libonnxruntime.1.27.0.dylib"},
		"linux":   {"libsherpa-onnx-c-api.so", "libonnxruntime.so"},
	}
	for _, name := range libs[runtime.GOOS] {
		dst := filepath.Join(dir, name)
		if _, err := os.Stat(dst); err == nil {
			fmt.Printf("Runtime lib: %s\n", dst)
			continue
		}
		if sherpaDir := findSherpaDir(); sherpaDir != "" {
			if copySherpaLib(sherpaDir, name, dst) {
				fmt.Printf("Runtime lib: %s\n", dst)
			}
		}
		if baseURL != "" {
			downloadAsset(baseURL, name, dir)
		}
	}
}

func copySherpaLib(sherpaDir, name, dst string) bool {
	libDir := filepath.Join(sherpaDir, "lib")
	entries, _ := os.ReadDir(libDir)

	// Map runtime.GOARCH to sherpa-onnx lib subdirectory names.
	archSubstr := map[string]string{
		"amd64": "x86_64",
		"arm64": "aarch64",
	}[runtime.GOARCH]

	// First pass: prefer the subdirectory matching the current architecture.
	if archSubstr != "" {
		for _, e := range entries {
			if e.IsDir() && strings.Contains(e.Name(), archSubstr) {
				src := filepath.Join(libDir, e.Name(), name)
				if data, err := os.ReadFile(src); err == nil {
					os.WriteFile(dst, data, 0755)
					return true
				}
			}
		}
	}

	// Second pass (fallback): any subdirectory.
	for _, e := range entries {
		if e.IsDir() {
			src := filepath.Join(libDir, e.Name(), name)
			if data, err := os.ReadFile(src); err == nil {
				os.WriteFile(dst, data, 0755)
				return true
			}
		}
	}
	return false
}

func downloadAsset(baseURL, name, dir string) error {
	url := baseURL + "/" + name
	dest := filepath.Join(dir, name)
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	fmt.Printf("Downloading %s...\n", url)
	return service.SaveResource(url, dest)
}

func compileHelper(helperPath, modelsDir string) error {
	src := "scripts/helper.c"
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("helper source not found")
	}
	sherpaDir := findSherpaDir()
	if sherpaDir == "" {
		runCmd("go", "mod", "download")
		sherpaDir = findSherpaDir()
	}
	if sherpaDir == "" {
		return fmt.Errorf("sherpa-onnx headers not found")
	}
	libDir := findSherpaLibDir(sherpaDir)
	fmt.Printf("Compiling audio helper...\n")
	args := []string{"-shared", "-o", helperPath, "-I" + sherpaDir, src}
	if libDir != "" {
		args = append(args, "-L"+libDir, "-lsherpa-onnx-c-api")
	}
	switch runtime.GOOS {
	case "darwin":
		args = append(args, "-install_name", "@rpath/"+filepath.Base(helperPath), "-Wl,-rpath,@loader_path")
	case "linux":
		args = append(args, `-Wl,-rpath,$ORIGIN`)
	}
	if err := runCmd("gcc", args...); err != nil {
		return err
	}
	fmt.Printf("Installed: %s\n", helperPath)
	return nil
}

func findSherpaDir() string {
	gmc := os.Getenv("GOMODCACHE")
	if gmc == "" {
		home, _ := os.UserHomeDir()
		gmc = filepath.Join(home, "go", "pkg", "mod")
	}
	goos := map[string]string{"darwin": "macos", "linux": "linux", "windows": "windows"}[runtime.GOOS]
	dir := filepath.Join(gmc, "github.com", "k2-fsa", fmt.Sprintf("sherpa-onnx-go-%s@v1.13.4", goos))
	if _, err := os.Stat(filepath.Join(dir, "c-api.h")); err == nil {
		return dir
	}
	// Try with different version
	dir = filepath.Join(gmc, "github.com", "k2-fsa", fmt.Sprintf("sherpa-onnx-go-%s@v1.13.4", goos))
	entries, _ := os.ReadDir(filepath.Dir(dir))
	for _, e := range entries {
		if strings.Contains(e.Name(), "sherpa-onnx-go-"+goos) {
			p := filepath.Join(filepath.Dir(dir), e.Name())
			if _, err := os.Stat(filepath.Join(p, "c-api.h")); err == nil {
				return p
			}
		}
	}
	return ""
}

func findSherpaLibDir(sherpaDir string) string {
	libDir := filepath.Join(sherpaDir, "lib")
	entries, _ := os.ReadDir(libDir)
	for _, e := range entries {
		if e.IsDir() {
			return filepath.Join(libDir, e.Name())
		}
	}
	return ""
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// audioModelsDir returns the base directory for audio models.
// Default: ~/.config/aigc-cli/models/audio/
func audioModelsDir() string {
	// Use shared models directory if available from detect config
	if shared.Cfg != nil && shared.Cfg.Detect != nil && shared.Cfg.Detect.ModelsDir != "" {
		return filepath.Join(shared.Cfg.Detect.ModelsDir, "audio")
	}
	if shared.Cfg != nil && shared.Cfg.Background != nil && shared.Cfg.Background.ModelsDir != "" {
		return filepath.Join(shared.Cfg.Background.ModelsDir, "audio")
	}
	return filepath.Join(configDir(), "models", "audio")
}

func init() {
	audioCmd.AddCommand(audioInitCmd)
	audioInitCmd.Flags().StringSliceVar(&audioInitModel, "model", nil, "model ID(s) to download (repeatable: --model a --model b)")
	audioInitCmd.Flags().BoolVar(&audioInitList, "list", false, "list available models")
	audioInitCmd.Flags().BoolVar(&audioInitListInst, "list-installed", false, "list installed models")
	audioInitCmd.Flags().StringVar(&audioInitType, "type", "", "filter by type: asr, tts")
	audioInitCmd.Flags().StringVar(&audioInitLang, "lang", "", "filter by language code (e.g. zh, en)")
	audioInitCmd.Flags().BoolVar(&audioInitForce, "force", false, "re-download even if already installed")
	audioInitCmd.Flags().StringVar(&audioInitHFToken, "hf-token", "", "HuggingFace token for gated models")
	audioInitCmd.Flags().StringVar(&audioInitURL, "url", "", "download from arbitrary URL (use with --name)")
	audioInitCmd.Flags().StringVar(&audioInitName, "name", "", "model name for --url downloads")
	audioInitCmd.Flags().BoolVar(&audioInitListVoices, "list-voices", false, "list available voices for a model")
}
