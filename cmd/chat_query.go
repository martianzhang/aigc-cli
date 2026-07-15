package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/martianzhang/apimart-cli/internal/ideas"
)

type webFetchArgs struct {
	URL       string `json:"url"`
	MaxLength int    `json:"max_length"`
	Offset    int    `json:"offset"`
}

func executeIdeasSearch(argsJSON string) string {
	var args struct {
		Keywords string `json:"keywords"`
		Limit    int    `json:"limit"`
		Random   bool   `json:"random"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Limit <= 0 {
		args.Limit = 5
	}
	if args.Random {
		text, err := ideas.SearchRandom(args.Limit)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text
	}
	if args.Keywords == "" {
		return "Error: keywords is required (or set random=true for random ideas)"
	}
	text, err := ideas.SearchText(args.Keywords, args.Limit)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return text
}

func executeBalanceQuery(argsJSON string) string {
	var args struct {
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Scope == "" {
		args.Scope = "token"
	}
	text, err := getBalanceText(args.Scope)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return text
}

func executeTaskQuery(argsJSON string) string {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	text, err := queryTaskText(args.TaskID)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return text
}

func executeWebFetch(argsJSON string) string {
	var args webFetchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.URL == "" {
		return "Error: url is required"
	}
	if args.Offset < 0 {
		return "Error: offset must be non-negative"
	}
	if args.MaxLength <= 0 {
		args.MaxLength = 5000
	}
	if args.MaxLength > 50000 {
		args.MaxLength = 50000
	}

	// How much to read: offset + requested + buffer; cap at 200K
	readLen := args.Offset + args.MaxLength + 5000
	if readLen > 200000 {
		readLen = 200000
	}

	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer fetchCancel()
	httpReq, err := http.NewRequestWithContext(fetchCtx, "GET", args.URL, nil)
	if err != nil {
		return fmt.Sprintf("Error: invalid URL %s: %v", args.URL, err)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Sprintf("Error: failed to fetch %s: %v", args.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Sprintf("Error: %s returned status %d", args.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(readLen)))
	if err != nil {
		return fmt.Sprintf("Error: failed to read response: %v", err)
	}

	totalSize := len(body)
	if args.Offset >= totalSize {
		return fmt.Sprintf("Page has %d bytes, but offset %d is past the end. Use offset=0 to read from the beginning.", totalSize, args.Offset)
	}

	end := args.Offset + args.MaxLength
	if end > totalSize {
		end = totalSize
	}
	content := string(body[args.Offset:end])
	remaining := totalSize - end

	var b strings.Builder
	fmt.Fprintf(&b, "Content from %s\n", args.URL)
	fmt.Fprintf(&b, "Fetched: %d bytes\n", totalSize)
	fmt.Fprintf(&b, "Showing bytes %d-%d", args.Offset, end)
	if remaining > 0 {
		fmt.Fprintf(&b, " (%d bytes remaining)\n\n", remaining)
	} else {
		b.WriteString("\n\n")
	}
	b.WriteString(content)

	if remaining > 0 {
		fmt.Fprintf(&b, "\n\nContent truncated. Use web_fetch(url=%q, offset=%d) to read the next section.", args.URL, end)
	}

	return b.String()
}

func printToolArgs(argsJSON string) {
	if argsJSON == "" || argsJSON == "{}" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		raw := argsJSON
		if len(raw) > 120 {
			raw = raw[:120] + "..."
		}
		fmt.Fprintf(chatStderr, "  %s\r\n", raw)
		return
	}
	for k, v := range m {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "..."
		}
		fmt.Fprintf(chatStderr, "  %s=%s\r\n", k, s)
	}
}

func executeReadFile(argsJSON string) string {
	var args struct {
		Filepath string `json:"filepath"`
		Offset   int    `json:"offset"`
		Limit    int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Filepath == "" {
		return "Error: filepath is required"
	}
	if args.Offset < 0 {
		return "Error: offset must be non-negative"
	}
	if args.Limit <= 0 {
		args.Limit = 10000
	}
	if args.Limit > 100000 {
		args.Limit = 100000
	}

	fpath := strings.TrimPrefix(args.Filepath, "@")
	ext := strings.ToLower(filepath.Ext(fpath))
	switch ext {
	case ".txt", ".md", ".yaml", ".yml", ".json", ".go", ".py", ".js", ".ts",
		".css", ".html", ".sh", ".bash", ".toml", ".ini", ".cfg", ".conf",
		".xml", ".svg", ".env", ".example":
	default:
		return fmt.Sprintf("Error: cannot read %s files for security reasons", ext)
	}

	content, err := os.ReadFile(fpath)
	if err != nil {
		return fmt.Sprintf("Error: cannot read %s: %v", fpath, err)
	}

	totalSize := len(content)
	if args.Offset >= totalSize {
		return fmt.Sprintf("File %s has %d bytes, but offset %d is past the end. Use offset=0 to read from the beginning.", fpath, totalSize, args.Offset)
	}

	// Return the requested slice
	end := args.Offset + args.Limit
	if end > totalSize {
		end = totalSize
	}
	chunk := string(content[args.Offset:end])
	remaining := totalSize - end

	// Build response with continuation metadata
	var b strings.Builder
	fmt.Fprintf(&b, "File: %s\n", fpath)
	fmt.Fprintf(&b, "Size: %d bytes\n", totalSize)
	fmt.Fprintf(&b, "Showing bytes %d-%d", args.Offset, end)
	if remaining > 0 {
		fmt.Fprintf(&b, " (%d bytes remaining)\n\n", remaining)
	} else {
		b.WriteString("\n\n")
	}
	b.WriteString("```\n")
	b.WriteString(chunk)
	b.WriteString("\n```")

	if remaining > 0 {
		fmt.Fprintf(&b, "\n\nFile has more content. Use read_file(filepath=%q, offset=%d) to read the next %d bytes.", fpath, end, args.Limit)
	}

	return b.String()
}

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

func executeShellCommand(cmdLine string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if runtime.GOOS == "windows" {
		// Windows 默认输出编码为 GBK/CP936，需切换到 UTF-8 避免中文乱码
		switch {
		case hasExecutable("pwsh"):
			cmdLine = "$OutputEncoding = [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " + cmdLine
			cmd := exec.CommandContext(ctx, "pwsh", "-NoProfile", "-Command", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		case hasExecutable("powershell"):
			cmdLine = "$OutputEncoding = [Console]::OutputEncoding = [System.Text.Encoding]::UTF8; " + cmdLine
			cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		default:
			cmdLine = "chcp 65001 >NUL & " + cmdLine
			cmd := exec.CommandContext(ctx, "cmd", "/c", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		}
	} else {
		switch {
		case hasExecutable("zsh"):
			cmd := exec.CommandContext(ctx, "zsh", "-c", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		case hasExecutable("bash"):
			cmd := exec.CommandContext(ctx, "bash", "-c", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		default:
			cmd := exec.CommandContext(ctx, "sh", "-c", cmdLine)
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, string(out))
			}
			return string(out)
		}
	}
}

func hasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
