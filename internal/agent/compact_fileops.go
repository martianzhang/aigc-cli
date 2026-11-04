package agent

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

type fileOps struct {
	read    map[string]struct{}
	written map[string]struct{}
	edited  map[string]struct{}
}

func newFileOps() *fileOps {
	return &fileOps{
		read:    map[string]struct{}{},
		written: map[string]struct{}{},
		edited:  map[string]struct{}{},
	}
}

// addPrevious carries file lists forward from the previous compaction so a
// chain of compactions does not forget which files were touched early on.
func (f *fileOps) addPrevious(readFiles, modifiedFiles []string) {
	for _, p := range readFiles {
		if p != "" {
			f.read[p] = struct{}{}
		}
	}
	for _, p := range modifiedFiles {
		if p != "" {
			f.edited[p] = struct{}{}
		}
	}
}

func (f *fileOps) addMessages(messages []types.ChatMessage) {
	for _, m := range messages {
		if m.Role != "assistant" {
			continue
		}
		for _, tc := range m.ToolCalls {
			var args struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil || args.Path == "" {
				continue
			}
			switch tc.Function.Name {
			case "read":
				f.read[args.Path] = struct{}{}
			case "write":
				f.written[args.Path] = struct{}{}
			case "edit":
				f.edited[args.Path] = struct{}{}
			}
		}
	}
}

// lists returns sorted read-only and modified file lists. A file that was both
// read and modified is reported as modified only.
func (f *fileOps) lists() (readFiles, modifiedFiles []string) {
	modified := make(map[string]struct{}, len(f.written)+len(f.edited))
	for p := range f.written {
		modified[p] = struct{}{}
	}
	for p := range f.edited {
		modified[p] = struct{}{}
	}
	for p := range f.read {
		if _, ok := modified[p]; !ok {
			readFiles = append(readFiles, p)
		}
	}
	for p := range modified {
		modifiedFiles = append(modifiedFiles, p)
	}
	sort.Strings(readFiles)
	sort.Strings(modifiedFiles)
	return readFiles, modifiedFiles
}

// stripFileOperationBlocks removes previously appended file lists so an
// incremental summary does not accumulate duplicate <read-files> blocks.
func stripFileOperationBlocks(s string) string {
	for _, tag := range []string{"read-files", "modified-files"} {
		startTag, endTag := "<"+tag+">", "</"+tag+">"
		for {
			start := strings.Index(s, startTag)
			if start < 0 {
				break
			}
			end := strings.Index(s[start:], endTag)
			if end < 0 {
				break
			}
			s = s[:start] + s[start+end+len(endTag):]
		}
	}
	return strings.TrimSpace(s)
}

// formatFileOperations appends machine-readable file lists to a summary.
func formatFileOperations(readFiles, modifiedFiles []string) string {
	var b strings.Builder
	if len(readFiles) > 0 {
		b.WriteString("\n\n<read-files>\n")
		for _, p := range readFiles {
			b.WriteString(p)
			b.WriteByte('\n')
		}
		b.WriteString("</read-files>")
	}
	if len(modifiedFiles) > 0 {
		b.WriteString("\n\n<modified-files>\n")
		for _, p := range modifiedFiles {
			b.WriteString(p)
			b.WriteByte('\n')
		}
		b.WriteString("</modified-files>")
	}
	return b.String()
}
