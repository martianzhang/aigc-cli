package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// executeFindFiles finds files by name/pattern under a directory (safe, pure Go).
func executeFindFiles(argsJSON string) string {
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
