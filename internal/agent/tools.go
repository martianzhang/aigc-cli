package agent

import (
	"encoding/json"

	"github.com/martianzhang/aigc-cli/internal/types"
)

// ToolDefs is the built-in primitive tool surface exposed to the chat agent.
//
// Domain capabilities (image/video/music generation, OCR, watermark, KB, …) are
// deliberately NOT defined here: they are provided by MCP servers configured in
// `mcp.servers` (see docs/guide-chat.md). This keeps a single source of truth
// for domain tools.
//
// Read-only primitives (read/grep/find/ls) are enabled by default; privileged
// primitives (write/edit/bash/web_fetch/web_search) require an explicit
// `tools_enable` entry. See IsReadOnlyPrimitive / IsPrivilegedPrimitive.
var ToolDefs = []types.ToolDefinition{
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "read",
			Description: "Read the contents of a local text file (e.g. .txt, .md, .yaml, .json, .go). Use when the user asks about or references a local file. Supports pagination via offset/limit for large files.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filepath": {"type": "string", "description": "Path to the local file to read (relative to current directory or absolute)"},
					"offset": {"type": "integer", "description": "Byte offset to start reading from (default 0). Use this to read the next chunk — the response tells you the file size and what offset to use next."},
					"limit": {"type": "integer", "description": "Max bytes to return (default 10000, max 100000)"}
				},
				"required": ["filepath"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "write",
			Description: "Write content to a local file, creating parent directories and overwriting the file if it exists. Use only for new files or full rewrites; for surgical changes prefer edit. Requires `tools_enable` to list \"write\".",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filepath": {"type": "string", "description": "Path to the file to write (relative to current directory or absolute)"},
					"content": {"type": "string", "description": "Full content to write to the file"}
				},
				"required": ["filepath", "content"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "edit",
			Description: "Edit a local file by replacing exact text. old_string must match exactly (including whitespace) and occur exactly once. Use for precise, surgical edits. Requires `tools_enable` to list \"edit\".",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"filepath": {"type": "string", "description": "Path to the file to edit"},
					"old_string": {"type": "string", "description": "Exact text to find (must match exactly and be unique)"},
					"new_string": {"type": "string", "description": "Replacement text"}
				},
				"required": ["filepath", "old_string", "new_string"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "bash",
			Description: "Execute a shell command in the current working directory and return its combined stdout/stderr. Use for ls/grep/find/curl and other CLI operations. Requires `tools_enable` to list \"bash\".",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {"type": "string", "description": "Shell command to execute"}
				},
				"required": ["command"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "grep",
			Description: "Search files for a pattern (regex). Returns matching lines with file paths and line numbers. Searches the current directory by default. Use this when the user asks to find text in files, search code, or locate something in the project.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Regex pattern to search for (Go regexp syntax)"},
					"path": {"type": "string", "description": "Directory or file to search in (default: current directory)"},
					"include": {"type": "string", "description": "Glob pattern to filter files (e.g. '*.go', '**/*.md'). Only search files matching this pattern."},
					"ignore_case": {"type": "boolean", "description": "Case insensitive search (default: false)"},
					"context": {"type": "integer", "description": "Number of context lines before/after each match (default: 0)"},
					"max_matches": {"type": "integer", "description": "Max matches to return (default: 20, max: 100)"}
				},
				"required": ["pattern"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "find",
			Description: "Recursively find files by name/pattern under a directory (like fd/find). Supports glob patterns. Use this when the user asks to find files, list images in a folder, search for documents, or explore the project structure.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Glob or substring to match filenames against, e.g. \"*.png\", \"test*.go\", \"README\""},
					"path": {"type": "string", "description": "Root directory to search (default: current directory \".\")"},
					"max_results": {"type": "integer", "description": "Maximum results to return (default: 30, max: 100)"}
				},
				"required": ["pattern"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "ls",
			Description: "List the entries of a directory (non-recursive). Directories are shown with a trailing slash. Use this to explore a folder before deciding what to read.",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"path": {"type": "string", "description": "Directory to list (default: current directory \".\")"}
				}
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "web_fetch",
			Description: "Fetch a URL and return its content. HTML pages are converted to markdown (main content only) so the result is clean and token-efficient; non-HTML content (JSON, plain text) is returned as-is. Use this when you need to read web pages, check current information, or access online resources. Large results are paginated by character offset. Requires `tools_enable` to list \"web_fetch\".",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"url": {"type": "string", "description": "URL to fetch (http:// or https://)"},
					"max_length": {"type": "integer", "description": "Max characters to return (default 5000, max 50000)"},
					"offset": {"type": "integer", "description": "Character offset to start reading from (default 0). Use this to paginate through large results — the response tells you how many characters remain and what offset to use next."}
				},
				"required": ["url"]
			}`),
		},
	},
	{
		Type: "function",
		Function: types.ToolFunction{
			Name:        "web_search",
			Description: "Search the web and return a ranked list of result titles and URLs. Use this when you need current information or a set of candidate pages; follow up with web_fetch to read a specific result. The search provider is chosen by the user's config. Requires `tools_enable` to list \"web_search\".",
			Parameters: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "Search query"},
					"limit": {"type": "integer", "description": "Max results to return (default 5, max 10)"}
				},
				"required": ["query"]
			}`),
		},
	},
}

// readOnlyPrimitives are enabled by default (safe, no side effects).
var readOnlyPrimitives = map[string]bool{
	"read": true,
	"grep": true,
	"find": true,
	"ls":   true,
}

// privilegedPrimitives require an explicit `tools_enable` entry.
var privilegedPrimitives = map[string]bool{
	"write":      true,
	"edit":       true,
	"bash":       true,
	"web_fetch":  true,
	"web_search": true,
}

// IsPrimitive reports whether name is one of the built-in primitives.
func IsPrimitive(name string) bool {
	return readOnlyPrimitives[name] || privilegedPrimitives[name]
}

// IsReadOnlyPrimitive reports whether name is a default-on, side-effect-free primitive.
func IsReadOnlyPrimitive(name string) bool { return readOnlyPrimitives[name] }

// IsPrivilegedPrimitive reports whether name requires explicit opt-in.
func IsPrivilegedPrimitive(name string) bool { return privilegedPrimitives[name] }
