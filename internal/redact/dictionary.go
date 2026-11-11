package redact

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/types"
)

const maxDictionaryFileBytes = 10 << 20 // 10 MB

// DictionaryEngine redacts exact terms from redact.dictionary and
// redact.dictionary_files. It is the no-model fallback for organizations,
// project names and internal codes.
type DictionaryEngine struct {
	entries []types.RedactDictionaryEntry
}

// NewDictionaryEngine validates and builds the dictionary engine. baseDir is
// used to resolve relative dictionary_files paths (normally the directory of
// config.yaml).
func NewDictionaryEngine(cfg *types.RedactConfig, baseDir string) (*DictionaryEngine, error) {
	if cfg == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	out := make([]types.RedactDictionaryEntry, 0, len(cfg.Dictionary))
	add := func(value, typ string) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("dictionary value must not be empty")
		}
		typ = strings.ToUpper(strings.TrimSpace(typ))
		if typ == "" {
			typ = "SENSITIVE"
		}
		key := value + "\x00" + typ
		if seen[key] {
			return nil
		}
		seen[key] = true
		out = append(out, types.RedactDictionaryEntry{Value: value, Type: typ})
		return nil
	}
	for _, e := range cfg.Dictionary {
		if err := add(e.Value, e.Type); err != nil {
			return nil, fmt.Errorf("redact.dictionary: %w", err)
		}
	}
	for _, path := range cfg.DictionaryFiles {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		if err := loadDictionaryFile(path, add); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return &DictionaryEngine{entries: out}, nil
}

// Name implements Engine.
func (e *DictionaryEngine) Name() string { return "dictionary" }

// Detect finds every occurrence of every dictionary term.
func (e *DictionaryEngine) Detect(_ context.Context, doc *Document) (Result, error) {
	var spans []Span
	for _, entry := range e.entries {
		for start := 0; ; {
			i := strings.Index(doc.Text[start:], entry.Value)
			if i < 0 {
				break
			}
			s := start + i
			spans = append(spans, Span{
				Start: s, End: s + len(entry.Value), Type: entry.Type,
				Text: entry.Value, Rule: "dictionary", Source: SourceRule,
				Score: 1, Priority: 200,
			})
			start = s + len(entry.Value)
		}
	}
	return Result{Spans: MergeSpans(spans)}, nil
}

type addEntryFunc func(value, typ string) error

func loadDictionaryFile(path string, add addEntryFunc) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("read dictionary file %s: %w", path, err)
	}
	if info.Size() > maxDictionaryFileBytes {
		return fmt.Errorf("dictionary file %s exceeds %d MB", path, maxDictionaryFileBytes>>20)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read dictionary file %s: %w", path, err)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".tsv":
		return parseTSV(path, string(data), add)
	case ".csv":
		return parseCSV(path, string(data), add)
	case ".json":
		return parseJSON(path, data, add)
	default:
		return fmt.Errorf("dictionary file %s: unsupported extension (use .tsv, .csv or .json)", path)
	}
}

func parseTSV(path, data string, add addEntryFunc) error {
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	headerSeen := false
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if !headerSeen && isDictionaryHeader(parts) {
			headerSeen = true
			continue
		}
		headerSeen = true
		value := strings.TrimSpace(parts[0])
		typ := ""
		if len(parts) > 1 {
			typ = strings.TrimSpace(parts[1])
		}
		if err := add(value, typ); err != nil {
			return fmt.Errorf("dictionary file %s:%d: %w", path, i+1, err)
		}
	}
	return nil
}

func parseCSV(path, data string, add addEntryFunc) error {
	r := csv.NewReader(strings.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	rows, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("parse dictionary file %s: %w", path, err)
	}
	headerSeen := false
	for i, row := range rows {
		if len(row) == 0 {
			continue
		}
		if strings.TrimSpace(row[0]) == "" {
			continue
		}
		if !headerSeen && isDictionaryHeader(row) {
			headerSeen = true
			continue
		}
		headerSeen = true
		typ := ""
		if len(row) > 1 {
			typ = row[1]
		}
		if err := add(row[0], typ); err != nil {
			return fmt.Errorf("dictionary file %s:%d: %w", path, i+1, err)
		}
	}
	return nil
}

func parseJSON(path string, data []byte, add addEntryFunc) error {
	var entries []types.RedactDictionaryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		var wrapper struct {
			Entries []types.RedactDictionaryEntry `json:"entries"`
		}
		if err2 := json.Unmarshal(data, &wrapper); err2 != nil || len(wrapper.Entries) == 0 {
			return fmt.Errorf("parse dictionary file %s: %w", path, err)
		}
		entries = wrapper.Entries
	}
	for i, e := range entries {
		if err := add(e.Value, e.Type); err != nil {
			return fmt.Errorf("dictionary file %s: entry %d: %w", path, i+1, err)
		}
	}
	return nil
}

func isDictionaryHeader(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	first := strings.ToLower(strings.TrimSpace(fields[0]))
	if first != "value" && first != "term" && first != "原文" && first != "词条" {
		return false
	}
	if len(fields) == 1 {
		return true
	}
	second := strings.ToLower(strings.TrimSpace(fields[1]))
	return second == "type" || second == "类型"
}
