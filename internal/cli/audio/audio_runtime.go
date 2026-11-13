package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/ui"
)

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

	baseURL := "https://github.com/martianzhang/aigc-cli-models/releases/download/v1"

	if _, err := os.Stat(helperPath); err == nil {
		ui.Printf("Audio helper: %s\n", helperPath)
	} else if err := downloadAsset(baseURL, helperName, modelsDir); err == nil {
		ui.Printf("Audio helper: %s\n", helperPath)
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
			ui.Printf("Runtime lib: %s\n", dst)
			continue
		}
		if sherpaDir := findSherpaDir(); sherpaDir != "" {
			if copySherpaLib(sherpaDir, name, dst) {
				ui.Printf("Runtime lib: %s\n", dst)
			}
		}
		if baseURL != "" {
			if err := downloadAsset(baseURL, name, dir); err != nil {
				ui.Eprintf("Warning: download %s: %v\n", name, err)
			}
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
				if data, err := os.ReadFile(src); err == nil { // #nosec G703 -- library path from the local sherpa cache
					if err := os.WriteFile(dst, data, 0755); err != nil {
						return false
					}
					return true
				}
			}
		}
	}

	// Second pass (fallback): any subdirectory.
	for _, e := range entries {
		if e.IsDir() {
			src := filepath.Join(libDir, e.Name(), name)
			if data, err := os.ReadFile(src); err == nil { // #nosec G703 -- library path from the local sherpa cache
				if err := os.WriteFile(dst, data, 0755); err != nil {
					return false
				}
				return true
			}
		}
	}
	return false
}

func compileHelper(helperPath, _ string) error {
	src := "scripts/helper.c"
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("helper source not found")
	}
	sherpaDir := findSherpaDir()
	if sherpaDir == "" {
		_ = runCmd("go", "mod", "download")
		sherpaDir = findSherpaDir()
	}
	if sherpaDir == "" {
		return fmt.Errorf("sherpa-onnx headers not found")
	}
	libDir := findSherpaLibDir(sherpaDir)
	ui.Printf("Compiling audio helper...\n")
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
	ui.Printf("Installed: %s\n", helperPath)
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
	if _, err := os.Stat(filepath.Join(dir, "c-api.h")); err == nil { // #nosec G703 -- module cache path
		return dir
	}
	// Try with different version
	dir = filepath.Join(gmc, "github.com", "k2-fsa", fmt.Sprintf("sherpa-onnx-go-%s@v1.13.4", goos))
	entries, _ := os.ReadDir(filepath.Dir(dir))
	for _, e := range entries {
		if strings.Contains(e.Name(), "sherpa-onnx-go-"+goos) {
			p := filepath.Join(filepath.Dir(dir), e.Name())
			if _, err := os.Stat(filepath.Join(p, "c-api.h")); err == nil { // #nosec G703 -- module cache path
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
	cmd := exec.CommandContext(context.Background(), name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
