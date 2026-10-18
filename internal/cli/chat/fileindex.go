package chat

import (
	"bytes"
	"context"
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// maxFileIndex bounds the number of candidate files held for @-completion.
const maxFileIndex = 5000

// projectFiles returns candidate files for @-completion under root. It prefers
// `git ls-files` so .gitignore is honoured, and falls back to a filesystem walk
// that skips hidden entries.
func projectFiles(root string) []string {
	if files, ok := gitFiles(root); ok {
		return files
	}
	return walkFiles(root)
}

// gitFiles lists tracked plus untracked-but-not-ignored files via git. It
// returns ok=false when git is unavailable or root is not a repository.
func gitFiles(root string) ([]string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	files := splitFileIndex(out.String())
	return files, len(files) > 0
}

// walkFiles is the git-less fallback: a recursive walk that skips hidden files
// and directories.
func walkFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && strings.HasPrefix(name, ".") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) >= maxFileIndex {
			return fs.SkipAll
		}
		return nil
	})
	return files
}

func splitFileIndex(s string) []string {
	var files []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, line)
		if len(files) >= maxFileIndex {
			break
		}
	}
	return files
}
