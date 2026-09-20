package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// confinedOutputPath resolves an MCP tool's output_path argument to an
// absolute target that cannot escape the directories the tool may write in.
//
// An empty requested path returns "" — callers keep their existing default,
// which is always written next to inputPath (inside an allowed root).
//
// A non-empty path must resolve inside one of the allowed roots:
//
//   - cfg.Output, when configured
//   - the directory containing inputPath
//
// Relative paths are resolved against those roots (cfg.Output first). An
// existing symlink at the target is rejected so the write cannot truncate an
// unrelated file through it.
func confinedOutputPath(cfg *Config, requested, inputPath string) (string, error) {
	if requested == "" {
		return "", nil
	}

	roots := allowedOutputRoots(cfg, inputPath)
	for _, candidate := range outputPathCandidates(requested, roots) {
		if !pathWithinRoots(candidate, roots) {
			continue
		}
		if info, err := os.Lstat(candidate); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("output_path must not be a symlink")
		}
		return candidate, nil
	}
	return "", fmt.Errorf("output_path escapes allowed directories")
}

// allowedOutputRoots returns the directories an MCP tool may write into,
// cfg.Output first when configured.
func allowedOutputRoots(cfg *Config, inputPath string) []string {
	roots := make([]string, 0, 2)
	if cfg != nil && cfg.Output != "" {
		roots = append(roots, filepath.Clean(resolveAbsPath(cfg.Output)))
	}
	inputDir := filepath.Dir(filepath.Clean(resolveAbsPath(inputPath)))
	return append(roots, inputDir)
}

// outputPathCandidates expands a requested path into the absolute paths to
// test: the cleaned path itself when absolute, otherwise it joined onto each
// allowed root.
func outputPathCandidates(requested string, roots []string) []string {
	cleaned := filepath.Clean(requested)
	if filepath.IsAbs(cleaned) {
		return []string{cleaned}
	}
	candidates := make([]string, 0, len(roots))
	for _, root := range roots {
		candidates = append(candidates, filepath.Join(root, cleaned))
	}
	return candidates
}

// pathWithinRoots reports whether candidate is a root itself or lives under one.
func pathWithinRoots(candidate string, roots []string) bool {
	for _, root := range roots {
		if candidate == root || strings.HasPrefix(candidate, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
