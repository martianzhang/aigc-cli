package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// maxSummaryScanBytes bounds how much of a session file Summarize reads. A
// conversation with many large tool results can grow into the hundreds of
// megabytes; scanning all of it to render a picker would stall the TUI. When
// the cap is hit the summary falls back to the header and mtime and marks
// itself truncated.
const maxSummaryScanBytes = 8 * 1024 * 1024

// maxSummaryPreviewRunes caps the first-user-message preview in a Summary.
const maxSummaryPreviewRunes = 80

// Summary is the metadata needed to list and identify a session without
// resuming it. It is built by streaming the JSONL file, so it is safe to call
// for files written by a newer schema: unknown record types are ignored.
type Summary struct {
	ID        string
	Name      string
	Path      string
	Cwd       string
	Provider  string
	Model     string
	FirstUser string
	CreatedAt time.Time
	UpdatedAt time.Time
	Bytes     int64
	Messages  int
	Truncated bool
}

// DisplayName returns the session name, falling back to its id.
func (s Summary) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	return s.ID
}

// Count returns the number of .jsonl files in dir. It is a cheap directory
// scan (no file parsing) used to tell the user how many sessions exist when a
// listing shows only the newest page.
func Count(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read session directory %s: %w", dir, err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".jsonl") {
			n++
		}
	}
	return n, nil
}

// List returns summaries for the .jsonl sessions in dir, newest first. limit
// caps the number returned (0 or negative means no limit). Corrupt or
// unreadable files are skipped so one broken transcript cannot hide the rest.
func List(dir string, limit int) ([]Summary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read session directory %s: %w", dir, err)
	}

	type entryInfo struct {
		path    string
		modTime time.Time
	}
	candidates := make([]entryInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, entryInfo{
			path:    filepath.Join(dir, e.Name()),
			modTime: info.ModTime(),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})

	out := make([]Summary, 0, len(candidates))
	for _, c := range candidates {
		s, err := Summarize(c.path)
		if err != nil {
			continue
		}
		out = append(out, s)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Summarize reads one session file and returns its listing metadata. It
// tolerates malformed lines (a crash can leave a partial append), but a file
// without a valid header is rejected: without one it is not a session.
func Summarize(path string) (Summary, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Summary{}, fmt.Errorf("stat session %s: %w", path, err)
	}
	if info.IsDir() {
		return Summary{}, fmt.Errorf("session %s is a directory", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return Summary{}, fmt.Errorf("read session %s: %w", path, err)
	}
	defer f.Close()

	sum := Summary{
		Path:      path,
		Bytes:     info.Size(),
		UpdatedAt: info.ModTime(),
		Truncated: info.Size() > maxSummaryScanBytes,
	}

	var (
		sawHeader bool
		header    Header
		lastModel *ModelRecord
		name      string
		barrier   int64
		msgSeqs   []int64
	)
	scanner := bufio.NewScanner(io.LimitReader(f, maxSummaryScanBytes))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			continue
		}
		if !sawHeader {
			if probe.Type != typeHeader {
				return Summary{}, fmt.Errorf("session %s: first record is not a header", path)
			}
			if err := json.Unmarshal([]byte(line), &header); err != nil {
				return Summary{}, fmt.Errorf("session %s: parse header: %w", path, err)
			}
			if header.ID == "" {
				return Summary{}, fmt.Errorf("session %s: header has no id", path)
			}
			sawHeader = true
			sum.ID = header.ID
			sum.Cwd = header.Cwd
			sum.Provider = header.Provider
			sum.Model = header.Model
			sum.CreatedAt = header.CreatedAt
			continue
		}

		switch probe.Type {
		case typeMessage:
			var rec MessageRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			switch rec.Role {
			case "user":
				msgSeqs = append(msgSeqs, rec.Seq)
				if sum.FirstUser == "" {
					sum.FirstUser = previewText(rec.Content)
				}
			case "assistant":
				msgSeqs = append(msgSeqs, rec.Seq)
			}

		case typeCompaction:
			var rec CompactionRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			if rec.UpToSeq > barrier {
				barrier = rec.UpToSeq
			}

		case typeClear:
			var rec ClearRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			if rec.UpToSeq > barrier {
				barrier = rec.UpToSeq
			}

		case typeSessionInfo:
			var rec SessionInfoRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			name = rec.Name

		case typeModel:
			var rec ModelRecord
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				continue
			}
			recCopy := rec
			lastModel = &recCopy
		}
	}
	if err := scanner.Err(); err != nil {
		return Summary{}, fmt.Errorf("read session %s: %w", path, err)
	}
	if !sawHeader {
		return Summary{}, fmt.Errorf("session %s: missing header", path)
	}
	if lastModel != nil {
		sum.Provider = lastModel.Provider
		sum.Model = lastModel.Model
	}
	sum.Name = name
	for _, seq := range msgSeqs {
		if seq > barrier {
			sum.Messages++
		}
	}
	return sum, nil
}

// previewText collapses whitespace and truncates a message for one-line
// display. File blocks and image markers are not stripped here: they are rare
// in the first user message and would require importing the chat package's
// title helpers, which would invert the dependency direction.
func previewText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= maxSummaryPreviewRunes {
		return s
	}
	return strings.TrimSpace(string(runes[:maxSummaryPreviewRunes])) + "…"
}
