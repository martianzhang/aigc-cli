package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WriteFile writes content to a file, creating parent directories as needed.
// Privileged primitive (requires explicit tools_enable opt-in).
func WriteFile(argsJSON string) string {
	var args struct {
		Filepath string `json:"filepath"`
		Content  string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Filepath == "" {
		return "Error: filepath is required"
	}

	if dir := filepath.Dir(args.Filepath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Sprintf("Error: cannot create directory %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(args.Filepath, []byte(args.Content), 0o644); err != nil {
		return fmt.Sprintf("Error: cannot write %s: %v", args.Filepath, err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(args.Content), args.Filepath)
}

// EditFile replaces an exact, unique occurrence of old_string with new_string.
// Privileged primitive (requires explicit tools_enable opt-in).
func EditFile(argsJSON string) string {
	var args struct {
		Filepath  string `json:"filepath"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Filepath == "" {
		return "Error: filepath is required"
	}
	if args.OldString == "" {
		return "Error: old_string is required"
	}

	data, err := os.ReadFile(args.Filepath)
	if err != nil {
		return fmt.Sprintf("Error: cannot read %s: %v", args.Filepath, err)
	}
	content := string(data)

	switch strings.Count(content, args.OldString) {
	case 0:
		return fmt.Sprintf("Error: old_string not found in %s", args.Filepath)
	case 1:
		// ok
	default:
		return fmt.Sprintf("Error: old_string is not unique in %s; include more surrounding context", args.Filepath)
	}

	updated := strings.Replace(content, args.OldString, args.NewString, 1)
	if err := os.WriteFile(args.Filepath, []byte(updated), 0o644); err != nil {
		return fmt.Sprintf("Error: cannot write %s: %v", args.Filepath, err)
	}
	return fmt.Sprintf("Edited %s", args.Filepath)
}

// ListDir lists the entries of a directory (non-recursive).
func ListDir(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	root := args.Path
	if root == "" {
		root = "."
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Sprintf("Error: cannot list %s: %v", root, err)
	}
	if len(entries) == 0 {
		return fmt.Sprintf("%s is empty", root)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s:\n", root)
	for _, e := range entries {
		if e.IsDir() {
			fmt.Fprintf(&b, "  %s/\n", e.Name())
		} else {
			fmt.Fprintf(&b, "  %s\n", e.Name())
		}
	}
	return b.String()
}
