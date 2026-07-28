package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/knowledge"
)

// searchDoubao searches with the Doubao (火山引擎豆包搜索) API.
func searchDoubao(store *knowledge.Store, query string, project string, verbose bool) error {
	apiKey := resolveDoubaoAPIKey()
	if apiKey == "" {
		return fmt.Errorf("DOUBAO_API_KEY env var required (or configure web_search.doubao.api_key in config.yaml)")
	}

	payload := map[string]interface{}{
		"Query":      query,
		"SearchType": "web",
		"Count":      3,
	}
	bodyJSON, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "https://open.feedcoopapi.com/search_api/web_search",
		strings.NewReader(string(bodyJSON)))
	if err != nil {
		return fmt.Errorf("doubao request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("doubao search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("doubao API error %d: %s", resp.StatusCode, string(respBody))
	}

	var dbResp struct {
		ResponseMetadata *struct {
			RequestID string `json:"RequestId"`
			Error     *struct {
				CodeN   int    `json:"CodeN"`
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
		Result *struct {
			ResultCount int `json:"ResultCount"`
			WebResults  []struct {
				URL     string `json:"Url"`
				Title   string `json:"Title"`
				Content string `json:"Content"`
			} `json:"WebResults"`
		} `json:"Result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dbResp); err != nil {
		return fmt.Errorf("doubao parse: %w", err)
	}

	if dbResp.ResponseMetadata != nil && dbResp.ResponseMetadata.Error != nil {
		return fmt.Errorf("doubao API error %s: %s",
			dbResp.ResponseMetadata.Error.Code,
			dbResp.ResponseMetadata.Error.Message)
	}

	results := dbResp.Result
	if results == nil || len(results.WebResults) == 0 {
		if verbose {
			fmt.Fprintf(os.Stderr, "No Doubao search results found for %q\n", query)
		}
		return nil
	}

	chunker := knowledge.NewChunker(knowledge.DefaultChunkOptions())
	embedder := knowledge.NewHashEmbedder(384)

	if verbose {
		fmt.Fprintf(os.Stderr, "Found %d result(s) via Doubao\n", len(results.WebResults))
	}

	webHeader := func() {
		fmt.Println("\nFrom web search:")
	}

	for _, r := range results.WebResults {
		if r.URL == "" {
			continue
		}

		// Doubao API returns full article Content directly — no need to fetch.
		content := r.Content
		if content == "" {
			if verbose {
				fmt.Fprintf(os.Stderr, "  Skip %s: empty content\n", r.URL)
			}
			continue
		}

		docID := knowledge.Checksum(content)
		existing, _ := store.GetDocument(docID)

		if existing != nil {
			if verbose {
				fmt.Fprintf(os.Stderr, "    From store: %s\n", r.Title)
			}
			webHeader()
			if cached := tryReadDoc(docID); cached != "" {
				outputDoc(docID, r.Title, r.URL, cached)
			} else {
				outputDoc(docID, r.Title, r.URL, content)
			}
			continue
		}

		webHeader()

		if shouldAutoSave() {
			doc := &knowledge.Document{
				ID:       docID,
				URL:      r.URL,
				Title:    r.Title,
				Project:  project,
				Size:     int64(len(content)),
				Checksum: docID,
			}
			if err := store.SaveDocument(doc); err != nil {
				if verbose {
					fmt.Fprintf(os.Stderr, "    Error saving: %v\n", err)
				}
			} else {
				knowledge.SaveDocFile(kbBaseDir, project, docID, r.Title, content)

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
					if verbose {
						fmt.Fprintf(os.Stderr, "    Error saving chunks: %v\n", err)
					}
				}
				if verbose {
					fmt.Fprintf(os.Stderr, "    Saved to KB: %s\n", r.Title)
				}
			}
		}
		outputDoc(docID, r.Title, r.URL, content)
	}

	return nil
}

// resolveDoubaoAPIKey returns the Doubao API key from config or env.
func resolveDoubaoAPIKey() string {
	cfg := shared.Cfg
	if cfg == nil || cfg.WebSearch == nil {
		if loaded, err := config.Load(shared.CfgFile); err == nil {
			cfg = loaded
		}
	}
	if cfg != nil {
		if p, ok := cfg.WebSearch["doubao"]; ok && p != nil && p.APIKey != "" {
			return p.APIKey
		}
		for _, p := range cfg.WebSearch {
			if p != nil && p.Type == "doubao" && p.APIKey != "" {
				return p.APIKey
			}
		}
	}
	return os.Getenv("DOUBAO_API_KEY")
}
