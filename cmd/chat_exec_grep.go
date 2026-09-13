package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// grepArgs holds parsed arguments for the grep tool.
type grepArgs struct {
	Pattern    string
	Path       string
	Include    string
	IgnoreCase bool
	Context    int
	MaxMatches int
}

func executeGrep(argsJSON string) string {
	var raw struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		Include    string `json:"include"`
		IgnoreCase bool   `json:"ignore_case"`
		Context    int    `json:"context"`
		MaxMatches int    `json:"max_matches"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	args := grepArgs{
		Pattern:    raw.Pattern,
		Path:       raw.Path,
		Include:    raw.Include,
		IgnoreCase: raw.IgnoreCase,
		Context:    raw.Context,
		MaxMatches: raw.MaxMatches,
	}
	if args.Pattern == "" {
		return "Error: pattern is required"
	}

	searchPath := args.Path
	if searchPath == "" {
		var err error
		searchPath, err = filepath.Abs(".")
		if err != nil {
			return fmt.Sprintf("Error: cannot get current directory: %v", err)
		}
	}
	if args.MaxMatches <= 0 {
		args.MaxMatches = 20
	} else if args.MaxMatches > 100 {
		args.MaxMatches = 100
	}
	if args.Context < 0 {
		args.Context = 0
	} else if args.Context > 10 {
		args.Context = 10
	}

	if hasExecutable("rg") {
		return grepWithRipgrep(&args, searchPath)
	}
	if hasExecutable("grep") {
		return grepWithGrep(&args, searchPath)
	}
	return grepGoImpl(&args, searchPath)
}

func grepWithRipgrep(args *grepArgs, searchPath string) string {
	rgArgs := []string{"--no-heading", "--line-number", "--color", "never"}
	if args.IgnoreCase {
		rgArgs = append(rgArgs, "-i")
	}
	if args.Context > 0 {
		rgArgs = append(rgArgs, "-C", fmt.Sprintf("%d", args.Context))
	}
	if args.Include != "" {
		rgArgs = append(rgArgs, "-g", args.Include)
	}
	rgArgs = append(rgArgs, "-m", fmt.Sprintf("%d", args.MaxMatches))
	rgArgs = append(rgArgs, "--", args.Pattern, searchPath)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rg", rgArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
		}
		return fmt.Sprintf("rg error: %v\n%s", err, string(out))
	}
	if len(out) == 0 {
		return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
	}
	return string(out)
}

func grepWithGrep(args *grepArgs, searchPath string) string {
	grepArgs := []string{"-rn", "--color=never"}
	if args.IgnoreCase {
		grepArgs = append(grepArgs, "-i")
	}
	if args.Context > 0 {
		grepArgs = append(grepArgs, "-C", fmt.Sprintf("%d", args.Context))
	} else {
		grepArgs = append(grepArgs, "-m", fmt.Sprintf("%d", args.MaxMatches))
	}
	if args.Include != "" {
		grepArgs = append(grepArgs, "--include", args.Include)
	}
	grepArgs = append(grepArgs, "--binary-files=without-match", "--exclude-dir=.git")
	grepArgs = append(grepArgs, "-e", args.Pattern, searchPath)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "grep", grepArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
		}
		return fmt.Sprintf("grep error: %v\n%s", err, string(out))
	}
	if len(out) == 0 {
		return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
	}
	result := string(out)
	lines := strings.Split(result, "\n")
	if len(lines) > args.MaxMatches {
		lines = lines[:args.MaxMatches]
		result = strings.Join(lines, "\n") + "\n...(truncated, max matches reached)"
	}
	return result
}

func grepGoImpl(args *grepArgs, searchPath string) string {
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return fmt.Sprintf("Error: invalid regex pattern '%s': %v", args.Pattern, err)
	}

	isDir := false
	if fi, err := os.Stat(searchPath); err == nil {
		isDir = fi.IsDir()
	}

	type match struct {
		file    string
		lineNum int
		line    string
	}
	var matches []match

	walkFn := func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name != "." && name != ".." && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if args.Include != "" {
			if matched, _ := filepath.Match(args.Include, info.Name()); !matched {
				return nil
			}
		}
		data, readErr := os.ReadFile(fpath)
		if readErr != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, match{file: fpath, lineNum: i + 1, line: line})
				if len(matches) >= args.MaxMatches {
					return filepath.SkipAll
				}
			}
		}
		return nil
	}

	if isDir {
		filepath.Walk(searchPath, walkFn)
	} else {
		walkFn(searchPath, nil, nil)
	}

	if len(matches) == 0 {
		return fmt.Sprintf("No matches found for pattern %q in %s", args.Pattern, searchPath)
	}

	var b strings.Builder
	for _, m := range matches {
		fmt.Fprintf(&b, "%s:%d:%s\n", m.file, m.lineNum, m.line)
	}
	if len(matches) >= args.MaxMatches {
		b.WriteString("...(truncated, max matches reached)")
	}
	return b.String()
}
