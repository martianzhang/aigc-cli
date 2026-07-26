package knowledge

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FileType represents the type of a local file.
type FileType int

const (
	FileTypeUnknown FileType = iota
	FileTypeText
	FileTypeMarkdown
	FileTypePDF
	FileTypeDocx
	FileTypeCode
	FileTypeJSON
	FileTypeYAML
	FileTypeHTML
)

// DetectFileType detects the type of a file by extension.
func DetectFileType(path string) FileType {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md", ".markdown", ".mdown":
		return FileTypeMarkdown
	case ".txt":
		return FileTypeText
	case ".pdf":
		return FileTypePDF
	case ".docx":
		return FileTypeDocx
	case ".go", ".py", ".rs", ".ts", ".tsx", ".js", ".jsx", ".java",
		".c", ".cpp", ".h", ".hpp", ".rb", ".php", ".sh", ".bash",
		".swift", ".kt", ".scala":
		return FileTypeCode
	case ".json":
		return FileTypeJSON
	case ".yaml", ".yml":
		return FileTypeYAML
	case ".html", ".htm", ".xhtml":
		return FileTypeHTML
	default:
		return FileTypeUnknown
	}
}

// LoadFile reads a file and returns its content as markdown text.
// For non-markdown files, it converts the content to markdown format.
func LoadFile(path string) (string, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read file: %w", err)
	}

	ft := DetectFileType(path)
	title := filepath.Base(path)

	var content string
	switch ft {
	case FileTypeText, FileTypeMarkdown:
		content = string(data)

	case FileTypeCode:
		content = fmt.Sprintf("```%s\n%s\n```", filepath.Ext(path)[1:], string(data))

	case FileTypeJSON:
		content = fmt.Sprintf("```json\n%s\n```", string(data))

	case FileTypeYAML:
		content = fmt.Sprintf("```yaml\n%s\n```", string(data))

	case FileTypeHTML:
		content = fmt.Sprintf("Source: %s\n\n```html\n%s\n```", path, string(data))

	case FileTypePDF:
		return "", "", fmt.Errorf("PDF support requires 'aigc-cli ocr scan': convert PDF to text first, then add the text file")

	case FileTypeDocx:
		return "", "", fmt.Errorf("DOCX support requires officecli: convert to markdown first, then add the text file")

	default:
		// Try as plain text
		if isText(data) {
			content = string(data)
		} else {
			return "", "", fmt.Errorf("unsupported file type: %s", filepath.Ext(path))
		}
	}

	return title, content, nil
}

// isText checks if data appears to be text (not binary).
func isText(data []byte) bool {
	// Check for null bytes (binary file signal)
	for _, b := range data {
		if b == 0 {
			return false
		}
		if b > 127 {
			// Could be UTF-8 multi-byte, that's fine
			continue
		}
	}
	return true
}

// LoadDir returns all supported files recursively in a directory.
func LoadDir(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		ft := DetectFileType(path)
		if ft != FileTypeUnknown {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// RunExternalLoader runs a configured external command to load a file.
// cmdTemplate is like "pdftotext $1 -", where $1 is replaced by the file path.
// Returns the command's stdout as content.
func RunExternalLoader(cmdTemplate, filePath string) (string, string, error) {
	cmdStr := strings.ReplaceAll(cmdTemplate, "$1", filePath)
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return "", "", fmt.Errorf("empty loader command")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("loader %q: %w", cmdStr, err)
	}
	title := filepath.Base(filePath)
	content := string(out)
	return title, content, nil
}
