package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// FindFiles finds files by name/pattern under a directory (safe, pure Go).
func FindFiles(argsJSON string) string {
	var params struct {
		Pattern   string `json:"pattern"`
		Path      string `json:"path"`
		MaxResult int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if params.Pattern == "" {
		return "Error: pattern is required"
	}
	root := params.Path
	if root == "" {
		root = "."
	}
	maxResults := params.MaxResult
	if maxResults <= 0 {
		maxResults = 30
	} else if maxResults > 100 {
		maxResults = 100
	}

	var results []string
	filepath.Walk(root, func(fpath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// Skip hidden directories
			if info.Name() != "." && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		matched, _ := filepath.Match(params.Pattern, info.Name())
		if !matched {
			matched = strings.Contains(strings.ToLower(info.Name()), strings.ToLower(params.Pattern))
		}
		if matched {
			size := info.Size()
			sizeStr := fmt.Sprintf("%d B", size)
			if size > 1024*1024 {
				sizeStr = fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
			} else if size > 1024 {
				sizeStr = fmt.Sprintf("%.1f KB", float64(size)/1024)
			}
			results = append(results, fmt.Sprintf("%s  (%s, %s)", fpath, sizeStr, info.ModTime().Format("2006-01-02")))
		}
		return nil
	})

	if len(results) == 0 {
		return fmt.Sprintf("No files found matching %q under %s", params.Pattern, root)
	}
	header := fmt.Sprintf("Found %d file(s) matching %q under %s:\n", len(results), params.Pattern, root)
	return header + strings.Join(results, "\n")
}

// ReadFile reads a local file with no additional access policy (all files allowed).
func ReadFile(argsJSON string) string {
	return ReadFileWithPolicy(argsJSON, nil)
}

// ReadFileRaw reads path as raw bytes after applying the read denylist
// (defaults.chat.read_deny). Unlike ReadFileWithPolicy it returns the bytes
// directly and surfaces an error, so callers that inline file content (e.g.
// chat @-mentions) can distinguish a policy block from a missing file.
func ReadFileRaw(path string, deny []string) ([]byte, error) {
	if deniedByReadPolicy(path, deny) {
		return nil, fmt.Errorf("reading %q is blocked by defaults.chat.read_deny", path)
	}
	return os.ReadFile(path)
}

// LooksBinary reports whether data is likely binary — a NUL byte, or more than
// ~1% invalid UTF-8 sequences in the first 8 KiB — and so should not be inlined
// into the model's context as text.
func LooksBinary(data []byte) bool { return looksBinary(data) }

// ReadFileWithPolicy reads a local file, rejecting any path that matches one of
// the deny regular expressions (matched against both the full path and the base
// name). An empty deny list allows every file.
func ReadFileWithPolicy(argsJSON string, deny []string) string {
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
	if deniedByReadPolicy(fpath, deny) {
		return fmt.Sprintf("Error: reading %q is blocked by defaults.chat.read_deny", fpath)
	}

	content, err := os.ReadFile(fpath)
	if err != nil {
		return fmt.Sprintf("Error: cannot read %s: %v", fpath, err)
	}

	totalSize := len(content)
	if looksBinary(content) {
		return fmt.Sprintf("File: %s\nSize: %d bytes\n\n[binary file — not displayed as text]", fpath, totalSize)
	}
	if args.Offset >= totalSize {
		return fmt.Sprintf("File %s has %d bytes, but offset %d is past the end. Use offset=0 to read from the beginning.", fpath, totalSize, args.Offset)
	}

	// Return the requested slice
	end := min(args.Offset+args.Limit, totalSize)
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
		fmt.Fprintf(&b, "\n\nFile has more content. Use read(filepath=%q, offset=%d) to read the next %d bytes.", fpath, end, args.Limit)
	}

	return b.String()
}

// deniedByReadPolicy reports whether path matches any of the deny regular
// expressions (checked against both the full path and the base name).
func deniedByReadPolicy(path string, deny []string) bool {
	if len(deny) == 0 {
		return false
	}
	base := filepath.Base(path)
	for _, pattern := range deny {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}
		if re.MatchString(path) || re.MatchString(base) {
			return true
		}
	}
	return false
}

// looksBinary reports whether data is likely a binary file (so it should not be
// dumped into the model's context as text). It samples the first 8 KiB: a NUL
// byte, or more than ~1% invalid UTF-8 sequences, marks the file as binary.
func looksBinary(data []byte) bool {
	const sampleMax = 8000
	sample := data
	if len(sample) > sampleMax {
		sample = sample[:sampleMax]
	}
	n := len(sample)
	if n == 0 {
		return false
	}
	if bytes.IndexByte(sample, 0) >= 0 {
		return true
	}

	invalid := 0
	for len(sample) > 0 {
		r, size := utf8.DecodeRune(sample)
		if r == utf8.RuneError && size == 1 {
			invalid++
		}
		sample = sample[size:]
	}
	return invalid*100 > n // >1% invalid runes
}
