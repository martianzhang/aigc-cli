package chat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/martianzhang/aigc-cli/internal/agent"
	"github.com/martianzhang/aigc-cli/internal/cli/options"
	"github.com/martianzhang/aigc-cli/internal/client"
	"github.com/martianzhang/aigc-cli/internal/types"
)

func chatKbDir() string {
	return filepath.Join(options.ConfigDir(), "knowledge")
}

func resolveFileRefs(argsJSON string) string {
	var raw interface{}
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return argsJSON // not valid JSON, return as-is
	}
	resolved := resolveFileRef(raw)
	data, _ := json.Marshal(resolved)
	return string(data)
}

// resolveFileRef recursively walks a parsed JSON value and replaces any
// string starting with "@" by reading the referenced file.
func resolveFileRef(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		if strings.HasPrefix(val, "@") {
			path := val[1:] // strip the @ prefix
			content, err := os.ReadFile(path)
			if err != nil {
				return fmt.Sprintf("[Error: cannot read %s: %v]", path, err)
			}
			return string(content)
		}
		return val
	case map[string]interface{}:
		for k, nested := range val {
			val[k] = resolveFileRef(nested)
		}
		return val
	case []interface{}:
		for i, item := range val {
			val[i] = resolveFileRef(item)
		}
		return val
	default:
		return val
	}
}

// executeToolCall executes a single tool call and returns a text result for the LLM.
func executeToolCall(c *client.Client, tc types.ToolCall) string {
	// Expand @filename references in arguments before dispatching.
	// Skip read_file — it handles its own file access.
	args := tc.Function.Arguments
	if tc.Function.Name != "read_file" && strings.Contains(args, `"@`) {
		resolved := resolveFileRefs(args)
		if resolved != args {
			if options.Shared.Verbose {
				fmt.Fprintf(options.Stderr(), "\r\n[agent] resolved @file refs in %s\r\n", tc.Function.Name)
			}
			args = resolved
		}
	}

	switch tc.Function.Name {
	case "generate_image":
		return executeGenerateImage(c, args)
	case "generate_video":
		return executeGenerateVideo(c, args)
	case "generate_music":
		return executeGenerateMusic(c, args)
	case "midjourney_imagine", "midjourney_describe", "midjourney_reroll", "midjourney_video":
		return executeMidjourney(c, tc.Function.Name, args)
	case "search_ideas":
		return executeIdeasSearch(args)
	case "balance":
		return executeBalanceQuery(args)
	case "task":
		return executeTaskQuery(args)
	case "web_fetch":
		return executeWebFetch(args)
	case "grep":
		return agent.Grep(args)
	case "read_file":
		return agent.ReadFile(args)
	case "find":
		return agent.FindFiles(args)
	case "remove_background":
		return executeRemoveBackground(args)
	case "convert_depth":
		return executeConvertDepth(args)
	case "remove_watermark":
		return executeRemoveWatermark(args)
	case "add_watermark":
		return executeAddWatermark(args)
	case "generate_speech":
		return executeGenerateSpeech(args)
	case "transcribe_audio":
		return executeTranscribeAudio(args)
	case "caption_image":
		return executeCaptionImage(args)
	case "recognize_text":
		return executeRecognizeText(args)
	case "kb_find":
		return agent.KbFind(chatKbDir(), args)
	case "kb_search":
		return agent.KbSearch(chatKbDir(), args)
	case "kb_add":
		return agent.KbAdd(chatKbDir(), args)
	case "kb_fetch":
		return agent.KbFetch(chatKbDir(), args)
	case "kb_list":
		return agent.KbList(chatKbDir(), args)
	case "kb_show":
		return agent.KbShow(chatKbDir(), args)
	default:
		return fmt.Sprintf("Error: unknown tool '%s'", tc.Function.Name)
	}
}

// summarizeToolResult returns a one-line summary of a tool's result for user display.

func summarizeToolResult(toolName, result string) string {
	// Truncate long results to first meaningful line
	firstLine := result
	if idx := strings.IndexAny(result, "\n\r"); idx > 0 {
		firstLine = result[:idx]
	}
	// Strip common prefixes for cleaner display
	firstLine = strings.TrimSpace(firstLine)
	if strings.HasPrefix(firstLine, "Successfully generated") {
		return firstLine
	}
	if strings.HasPrefix(firstLine, "Error:") {
		return firstLine
	}
	if len(firstLine) > 80 {
		firstLine = firstLine[:80] + "..."
	}
	return firstLine
}
