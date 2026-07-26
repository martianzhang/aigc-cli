package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/martianzhang/aigc-cli/internal/knowledge"
)

func kbDir() string {
	return knowledgeBaseDir()
}

func knowledgeBaseDir() string {
	return "~/.config/aigc-cli/knowledge"
}

func openKBStore() (*knowledge.Store, error) {
	return knowledge.OpenStore(kbDir(), 384, nil)
}

// ----- Tool definitions -----

func newKbFindTool() mcp.Tool {
	return mcp.NewTool("kb_find",
		mcp.WithDescription("Search the local knowledge base using keyword and semantic search. Use this when you need to recall information that was previously saved to the knowledge base."),
		mcp.WithString("query",
			mcp.Description("Search query"),
			mcp.Required(),
		),
		mcp.WithInteger("limit",
			mcp.Description("Max results (default 10)"),
		),
	)
}

func newKbSearchTool() mcp.Tool {
	return mcp.NewTool("kb_search",
		mcp.WithDescription("Search the web and save results to the local knowledge base. Use this to research a topic and save it for later recall."),
		mcp.WithString("query",
			mcp.Description("Search query"),
			mcp.Required(),
		),
		mcp.WithString("provider",
			mcp.Description("Search provider: duckduckgo (default) or firecrawl"),
		),
	)
}

func newKbAddTool() mcp.Tool {
	return mcp.NewTool("kb_add",
		mcp.WithDescription("Add a local file to the knowledge base. Supports: .md, .txt, .go, .py, .json, .yaml, .html."),
		mcp.WithString("file_path",
			mcp.Description("Path to the local file"),
			mcp.Required(),
		),
	)
}

func newKbFetchTool() mcp.Tool {
	return mcp.NewTool("kb_fetch",
		mcp.WithDescription("Fetch a URL, extract its main content, convert to markdown, and save to the knowledge base."),
		mcp.WithString("url",
			mcp.Description("URL to fetch"),
			mcp.Required(),
		),
	)
}

func newKbShowTool() mcp.Tool {
	return mcp.NewTool("kb_show",
		mcp.WithDescription("Show the full content of a document in the knowledge base by its ID. Use the ID returned by kb_find."),
		mcp.WithString("doc_id",
			mcp.Description("Document ID (first 12 characters, as shown in kb_find results)"),
			mcp.Required(),
		),
	)
}

func newKbListTool() mcp.Tool {
	return mcp.NewTool("kb_list",
		mcp.WithDescription("List all documents in the knowledge base."),
	)
}

func argsMap(req mcp.CallToolRequest) (map[string]interface{}, error) {
	args, ok := req.Params.Arguments.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid arguments")
	}
	return args, nil
}

// ----- Handlers -----

func kbFindHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := argsMap(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		query, _ := args["query"].(string)
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}
		limit := 10
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}

		store, err := openKBStore()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("open store: %v", err)), nil
		}
		defer store.Close()

		results, err := store.Search(query, limit*3, "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search: %v", err)), nil
		}

		if len(results) == 0 {
			return mcp.NewToolResultText("No results found in the knowledge base. Try kb_search to search the web and save results first."), nil
		}

		// Aggregate by document
		type docRes struct {
			title   string
			source  string
			docID   string
			score   float64
			nchunk  int
			summary string
		}
		docMap := make(map[string]*docRes)
		for _, r := range results {
			d, ok := docMap[r.Document.ID]
			if !ok {
				source := r.Document.URL
				if source == "" {
					source = r.Document.FilePath
				}
				docMap[r.Document.ID] = &docRes{
					title:   r.Document.Title,
					source:  source,
					docID:   r.Document.ID[:12],
					score:   r.Score,
					summary: firstParagraph(r.Chunk.Content),
				}
				d = docMap[r.Document.ID]
			}
			if r.Score > d.score {
				d.score = r.Score
			}
			if d.summary == "" {
				d.summary = firstParagraph(r.Chunk.Content)
			}
			d.nchunk++
		}

		docs := make([]*docRes, 0, len(docMap))
		for _, d := range docMap {
			docs = append(docs, d)
		}
		// Sort by score
		for i := 0; i < len(docs); i++ {
			for j := i + 1; j < len(docs); j++ {
				if docs[j].score > docs[i].score {
					docs[i], docs[j] = docs[j], docs[i]
				}
			}
		}

		if limit > len(docs) {
			limit = len(docs)
		}
		docs = docs[:limit]

		var out strings.Builder
		fmt.Fprintf(&out, "Found %d matching document(s) in the knowledge base:\n", len(docs))
		for _, d := range docs {
			fmt.Fprintf(&out, "\n---\n")
			fmt.Fprintf(&out, "**%s**  \n", d.title)
			fmt.Fprintf(&out, "ID: `%s` | Score: %.4f (%d chunk(s))  \n", d.docID, d.score, d.nchunk)
			fmt.Fprintf(&out, "Source: %s  \n", d.source)
			fmt.Fprintf(&out, "Show: `kb_show %s`  \n", d.docID)
			if d.summary != "" {
				fmt.Fprintf(&out, "\n%s\n", d.summary)
			}
		}
		fmt.Fprintf(&out, "\n---\n")
		return mcp.NewToolResultText(out.String()), nil
	}
}

func kbSearchHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := argsMap(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		query, _ := args["query"].(string)
		if query == "" {
			return mcp.NewToolResultError("query is required"), nil
		}

		store, err := openKBStore()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("open store: %v", err)), nil
		}
		defer store.Close()

		urls, err := knowledge.DDGSearchURLs(query)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search request: %v", err)), nil
		}

		if len(urls) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf("No search results found for %q.", query)), nil
		}

		chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
		embedder := knowledge.NewHashEmbedder(384)
		out := fmt.Sprintf("Searched for %q, found and saved %d result(s):\n\n", query, len(urls))

		for _, rawURL := range urls {
			result, err := knowledge.FetchURL(rawURL)
			if err != nil {
				out += fmt.Sprintf("  \u274c %s (%v)\n", rawURL, err)
				continue
			}

			docID := knowledge.Checksum(result.Content)
			existing, _ := store.GetDocument(docID)
			if existing != nil {
				out += fmt.Sprintf("  \u2713 %s (already saved)\n", result.Title)
				continue
			}

			doc := &knowledge.Document{
				ID:       docID,
				URL:      result.URL,
				Title:    result.Title,
				Size:     result.Size,
				Checksum: docID,
			}
			if err := store.SaveDocument(doc); err != nil {
				out += fmt.Sprintf("  \u274c %s: save error: %v\n", result.Title, err)
				continue
			}

			knowledge.SaveDocFile(kbDir(), "", docID, result.Title, result.Content)

			rawChunks := chunker.Chunk(result.Content)
			embeddings := make([]knowledge.Embedding, len(rawChunks))
			for i, c := range rawChunks {
				emb, err := embedder.Embed(c.Content)
				if err != nil {
					continue
				}
				embeddings[i] = emb
			}
			if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
				out += fmt.Sprintf("  \u274c %s: chunk save error: %v\n", result.Title, err)
				continue
			}

			snippet := result.Content
			if len(snippet) > 500 {
				snippet = snippet[:500] + "..."
			}
			out += fmt.Sprintf("  \u2713 %s\n     %s\n     %s\n\n", result.Title, result.URL, snippet)
		}

		return mcp.NewToolResultText(out), nil
	}
}

func kbAddHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := argsMap(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		filePath, _ := args["file_path"].(string)
		if filePath == "" {
			return mcp.NewToolResultError("file_path is required"), nil
		}

		store, err := openKBStore()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("open store: %v", err)), nil
		}
		defer store.Close()

		title, content, err := knowledge.LoadFile(filePath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("load file: %v", err)), nil
		}

		checksum := knowledge.Checksum(content)
		docID := checksum

		existing, _ := store.GetDocument(docID)
		if existing != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Already in KB: %s", title)), nil
		}

		doc := &knowledge.Document{
			ID:       docID,
			FilePath: filePath,
			Title:    title,
			Size:     int64(len(content)),
			Checksum: checksum,
		}
		if err := store.SaveDocument(doc); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("save doc: %v", err)), nil
		}
		knowledge.SaveDocFile(kbDir(), "", docID, title, content)

		chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
		embedder := knowledge.NewHashEmbedder(384)

		rawChunks := chunker.Chunk(content)
		embeddings := make([]knowledge.Embedding, len(rawChunks))
		for i, c := range rawChunks {
			emb, err := embedder.Embed(c.Content)
			if err != nil {
				continue
			}
			embeddings[i] = emb
		}
		if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("save chunks: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Added %q to the knowledge base (%d chunks).", title, len(rawChunks))), nil
	}
}

func kbFetchHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := argsMap(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		rawURL, _ := args["url"].(string)
		if rawURL == "" {
			return mcp.NewToolResultError("url is required"), nil
		}

		result, err := knowledge.FetchURL(rawURL)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("fetch: %v", err)), nil
		}

		store, err := openKBStore()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("open store: %v", err)), nil
		}
		defer store.Close()

		docID := knowledge.Checksum(result.Content)
		existing, _ := store.GetDocument(docID)
		if existing != nil {
			return mcp.NewToolResultText(fmt.Sprintf("Already in KB: %s\n\nContent preview:\n\n%s",
				result.Title, truncate(result.Content, 1000))), nil
		}

		doc := &knowledge.Document{
			ID:       docID,
			URL:      result.URL,
			Title:    result.Title,
			Size:     result.Size,
			Checksum: docID,
		}
		if err := store.SaveDocument(doc); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("save: %v", err)), nil
		}
		knowledge.SaveDocFile(kbDir(), "", docID, result.Title, result.Content)

		chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
		embedder := knowledge.NewHashEmbedder(384)

		rawChunks := chunker.Chunk(result.Content)
		embeddings := make([]knowledge.Embedding, len(rawChunks))
		for i, c := range rawChunks {
			emb, err := embedder.Embed(c.Content)
			if err != nil {
				continue
			}
			embeddings[i] = emb
		}
		if err := store.SaveChunks(docID, rawChunks, embeddings, false); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("save chunks: %v", err)), nil
		}

		snippet := truncate(result.Content, 1000)
		return mcp.NewToolResultText(fmt.Sprintf(
			"\u2713 Saved to KB: %s\nTitle: %s\nChunks: %d\n\nPreview:\n%s",
			result.URL, result.Title, len(rawChunks), snippet)), nil
	}
}

func kbListHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		store, err := openKBStore()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("open store: %v", err)), nil
		}
		defer store.Close()

		docs, err := store.ListDocuments(100, 0, "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("list: %v", err)), nil
		}

		if len(docs) == 0 {
			return mcp.NewToolResultText("Knowledge base is empty. Use kb_search to save web pages or kb_add to add local files."), nil
		}

		out := fmt.Sprintf("Knowledge base: %d document(s)\n\n", len(docs))
		for i, d := range docs {
			source := d.URL
			if source == "" {
				source = d.FilePath
			}
			out += fmt.Sprintf("%d. %s\n   ID: %s\n   Source: %s\n   Size: %.1f KB\n\n",
				i+1, d.Title, d.ID[:12], source, float64(d.Size)/1024)
		}
		return mcp.NewToolResultText(out), nil
	}
}

func kbShowHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := argsMap(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		docID, _ := args["doc_id"].(string)
		if docID == "" {
			return mcp.NewToolResultError("doc_id is required"), nil
		}

		store, err := openKBStore()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("open store: %v", err)), nil
		}
		defer store.Close()

		// Try to find by prefix match
		doc, err := findDocByIDPrefix(store, docID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("error: %v", err)), nil
		}
		if doc == nil {
			return mcp.NewToolResultText(fmt.Sprintf("Document %q not found.", docID)), nil
		}

		// Read content from docs/ file
		content := tryReadDocFile(kbDir(), doc.ID)
		if content == "" {
			return mcp.NewToolResultText(fmt.Sprintf("Document %q found but content file is missing.", docID)), nil
		}

		out := fmt.Sprintf("Title: %s\nID: %s\n", doc.Title, doc.ID[:12])
		if doc.URL != "" {
			out += fmt.Sprintf("URL: %s\n", doc.URL)
		}
		if doc.FilePath != "" {
			out += fmt.Sprintf("File: %s\n", doc.FilePath)
		}
		out += "\n" + content

		return mcp.NewToolResultText(out), nil
	}
}

// findDocByIDPrefix looks up a document by ID prefix.
func findDocByIDPrefix(store *knowledge.Store, prefix string) (*knowledge.Document, error) {
	rows, err := store.DB().Query(
		"SELECT id FROM documents WHERE id LIKE ? || '%'", prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fullID string
	if rows.Next() {
		if err := rows.Scan(&fullID); err != nil {
			return nil, err
		}
	}
	if rows.Next() {
		return nil, fmt.Errorf("ambiguous prefix: multiple documents match %q", prefix)
	}
	if fullID == "" {
		return nil, nil
	}
	return store.GetDocument(fullID)
}

// findDocByIDPrefix looks up a document by ID prefix.
func tryReadDocFile(baseDir, docID string) string {
	docsDir := filepath.Join(baseDir, "docs")
	var found string
	filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), docID[:12]) {
			found = path
			return fmt.Errorf("stop")
		}
		return nil
	})
	if found == "" {
		return ""
	}
	data, err := os.ReadFile(found)
	if err != nil {
		return ""
	}
	return string(data)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// firstParagraph returns a short preview from the start of content.
func firstParagraph(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	maxLen := 300
	if len(s) <= maxLen {
		return s
	}
	if idx := strings.Index(s[:maxLen], "\n"); idx > 50 {
		return s[:idx]
	}
	if idx := strings.LastIndex(s[:maxLen], ". "); idx > 50 {
		return s[:idx+1]
	}
	return s[:maxLen] + "..."
}
