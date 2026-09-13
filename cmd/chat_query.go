package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/martianzhang/aigc-cli/internal/ideas"
)

type webFetchArgs struct {
	URL       string `json:"url"`
	MaxLength int    `json:"max_length"`
	Offset    int    `json:"offset"`
}

func executeIdeasSearch(argsJSON string) string {
	var args struct {
		Keywords string `json:"keywords"`
		Limit    int    `json:"limit"`
		Random   bool   `json:"random"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Limit <= 0 {
		args.Limit = 5
	}
	if args.Random {
		text, err := ideas.SearchRandom(args.Limit)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return text
	}
	if args.Keywords == "" {
		return "Error: keywords is required (or set random=true for random ideas)"
	}
	text, err := ideas.SearchText(args.Keywords, args.Limit)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return text
}

func executeBalanceQuery(argsJSON string) string {
	var args struct {
		Scope string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.Scope == "" {
		args.Scope = "token"
	}
	text, err := getBalanceText(args.Scope)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return text
}

func executeTaskQuery(argsJSON string) string {
	var args struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	text, err := queryTaskText(args.TaskID)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return text
}

func executeWebFetch(argsJSON string) string {
	var args webFetchArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Sprintf("Error: invalid arguments: %v", err)
	}
	if args.URL == "" {
		return "Error: url is required"
	}
	if args.Offset < 0 {
		return "Error: offset must be non-negative"
	}
	if args.MaxLength <= 0 {
		args.MaxLength = 5000
	}
	if args.MaxLength > 50000 {
		args.MaxLength = 50000
	}

	// How much to read: offset + requested + buffer; cap at 200K
	readLen := args.Offset + args.MaxLength + 5000
	if readLen > 200000 {
		readLen = 200000
	}

	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer fetchCancel()
	httpReq, err := http.NewRequestWithContext(fetchCtx, "GET", args.URL, nil)
	if err != nil {
		return fmt.Sprintf("Error: invalid URL %s: %v", args.URL, err)
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return fmt.Sprintf("Error: failed to fetch %s: %v", args.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return fmt.Sprintf("Error: %s returned status %d", args.URL, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(readLen)))
	if err != nil {
		return fmt.Sprintf("Error: failed to read response: %v", err)
	}

	totalSize := len(body)
	if args.Offset >= totalSize {
		return fmt.Sprintf("Page has %d bytes, but offset %d is past the end. Use offset=0 to read from the beginning.", totalSize, args.Offset)
	}

	end := args.Offset + args.MaxLength
	if end > totalSize {
		end = totalSize
	}
	content := string(body[args.Offset:end])
	remaining := totalSize - end

	var b strings.Builder
	fmt.Fprintf(&b, "Content from %s\n", args.URL)
	fmt.Fprintf(&b, "Fetched: %d bytes\n", totalSize)
	fmt.Fprintf(&b, "Showing bytes %d-%d", args.Offset, end)
	if remaining > 0 {
		fmt.Fprintf(&b, " (%d bytes remaining)\n\n", remaining)
	} else {
		b.WriteString("\n\n")
	}
	b.WriteString(content)

	if remaining > 0 {
		fmt.Fprintf(&b, "\n\nContent truncated. Use web_fetch(url=%q, offset=%d) to read the next section.", args.URL, end)
	}

	return b.String()
}

func printToolArgs(argsJSON string) {
	if argsJSON == "" || argsJSON == "{}" {
		return
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		raw := argsJSON
		if len(raw) > 120 {
			raw = raw[:120] + "..."
		}
		fmt.Fprintf(chatStderr, "  %s\r\n", raw)
		return
	}
	for k, v := range m {
		s := fmt.Sprintf("%v", v)
		if len(s) > 80 {
			s = s[:80] + "..."
		}
		fmt.Fprintf(chatStderr, "  %s=%s\r\n", k, s)
	}
}
