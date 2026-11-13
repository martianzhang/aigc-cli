package docconvert

import (
	"fmt"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/webfetch"
)

// fromHTML converts one local HTML file to Markdown using the same
// readability pipeline as `aigc-cli fetch`.
func fromHTML(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read HTML: %w", err)
	}
	title, markdown, err := webfetch.HTMLToMarkdown(string(data), nil)
	if err != nil {
		return "", fmt.Errorf("convert HTML: %w", err)
	}
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return "", fmt.Errorf("HTML contains no readable text")
	}
	if title = strings.TrimSpace(title); title != "" && !strings.HasPrefix(markdown, "#") {
		markdown = "# " + title + "\n\n" + markdown
	}
	return markdown + "\n", nil
}
