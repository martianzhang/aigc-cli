package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
)

func KbList(kbDir, argsJSON string) string {
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	store, err := knowledge.OpenStore(kbDir, 384, nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer store.Close()

	docs, err := store.ListDocuments(100, 0, "")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	if len(docs) == 0 {
		return "Knowledge base is empty."
	}

	var out strings.Builder
	fmt.Fprintf(&out, "Knowledge base: %d document(s)\n\n", len(docs))
	for _, d := range docs {
		source := d.URL
		if source == "" {
			source = d.FilePath
		}
		id := d.ID[:12]
		fmt.Fprintf(&out, "  %s  %-30s  %s\n", id, d.Title, source)
	}
	return out.String()
}

func KbShow(kbDir, argsJSON string) string {
	var args struct {
		DocID string `json:"doc_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.DocID == "" {
		return "Error: doc_id is required"
	}

	if err := os.MkdirAll(kbDir, 0755); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	store, err := knowledge.OpenStore(kbDir, 384, nil)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	defer store.Close()

	rows, err := store.DB().Query("SELECT id FROM documents WHERE id LIKE ? || '%'", args.DocID)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		return fmt.Sprintf("Document %q not found.", args.DocID)
	}
	if len(ids) > 1 {
		return fmt.Sprintf("Ambiguous: %q matches %d documents. Use a longer prefix.", args.DocID, len(ids))
	}

	doc, err := store.GetDocument(ids[0])
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if doc == nil {
		return fmt.Sprintf("Document %q not found.", args.DocID)
	}

	var content string
	docsDir := filepath.Join(kbDir, "docs")
	filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), doc.ID[:12]) {
			data, _ := os.ReadFile(path)
			content = string(data)
			return fmt.Errorf("stop")
		}
		return nil
	})

	var out strings.Builder
	fmt.Fprintf(&out, "Title: %s\nID: %s\n", doc.Title, doc.ID[:12])
	if doc.URL != "" {
		fmt.Fprintf(&out, "URL: %s\n", doc.URL)
	}
	if doc.FilePath != "" {
		fmt.Fprintf(&out, "File: %s\n", doc.FilePath)
	}
	fmt.Fprintf(&out, "\n%s\n", content)
	return out.String()
}
