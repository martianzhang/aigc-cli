package mcp

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// outputURIPrefix is the concrete URI prefix matching outputTemplateURI, and
// maxResourceFileSize caps how large a single file may be before reading (4 MiB).
const (
	outputURIPrefix     = "aigc://output/"
	maxResourceFileSize = 4 << 20
)

// outputFileHandler reads one file from the configured output directory.
func outputFileHandler(cfg *Config) server.ResourceTemplateHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		name, ok := outputNameFromURI(req.Params.URI)
		if !ok {
			return nil, fmt.Errorf("unsupported resource URI %q", req.Params.URI)
		}
		kind, mimeType := classifyResource(name)
		if kind == resourceUnsupported {
			return nil, fmt.Errorf("unsupported resource type: %q", filepath.Ext(name))
		}
		full, err := resolveOutputPath(cfg.Output, name)
		if err != nil {
			return nil, err
		}

		info, err := os.Lstat(full)
		if err != nil {
			return nil, fmt.Errorf("resource not found: %s", name)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing to follow symlink: %s", name)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("not a regular file: %s", name)
		}
		if info.Size() > maxResourceFileSize {
			return nil, fmt.Errorf("resource too large: %s is %d bytes (limit %d)", name, info.Size(), maxResourceFileSize)
		}

		data, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("read resource %s: %w", name, err)
		}
		if kind == resourceText {
			return textContents(req, mimeType, string(data)), nil
		}
		return []mcp.ResourceContents{mcp.BlobResourceContents{
			URI:      req.Params.URI,
			MIMEType: mimeType,
			Blob:     base64.StdEncoding.EncodeToString(data),
		}}, nil
	}
}

// outputNameFromURI extracts the {filename} variable from an aigc://output/...
// URI. The raw value is never used as a path before resolveOutputPath validates it.
func outputNameFromURI(uri string) (string, bool) {
	if !strings.HasPrefix(uri, outputURIPrefix) {
		return "", false
	}
	return strings.TrimPrefix(uri, outputURIPrefix), true
}

// resolveOutputPath validates filename as a plain file name inside root and
// returns the cleaned path. It rejects empty names, path separators, parent
// references, absolute paths, and volume names, then double-checks that the
// cleaned join still has the cleaned root as a prefix.
func resolveOutputPath(root, filename string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("no output directory configured")
	}
	if filename == "" {
		return "", fmt.Errorf("filename is required")
	}
	if strings.Contains(filename, "..") || strings.ContainsAny(filename, `/\`) ||
		filepath.IsAbs(filename) || filepath.VolumeName(filename) != "" {
		return "", fmt.Errorf("invalid filename %q: only plain file names inside the output directory are allowed", filename)
	}
	cleanRoot := filepath.Clean(root)
	full := filepath.Clean(filepath.Join(cleanRoot, filename))
	if full != cleanRoot && !strings.HasPrefix(full, cleanRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid filename %q: outside the output directory", filename)
	}
	return full, nil
}

// resourceKind selects the contents block used for a resource file.
type resourceKind int

const (
	resourceUnsupported resourceKind = iota
	resourceBlob
	resourceText
)

// textResourceMIME maps extensions served as text resource contents.
var textResourceMIME = map[string]string{
	".txt":  "text/plain",
	".md":   "text/markdown",
	".json": "application/json",
	".yaml": "application/yaml",
	".yml":  "application/yaml",
	".csv":  "text/csv",
	".log":  "text/plain",
}

// classifyResource maps a file name to its contents kind and MIME type,
// reusing mediaMIME for image/audio blobs.
func classifyResource(filename string) (resourceKind, string) {
	ext := strings.ToLower(filepath.Ext(filename))
	if mimeType, ok := mediaMIME[ext]; ok {
		return resourceBlob, mimeType
	}
	if mimeType, ok := textResourceMIME[ext]; ok {
		return resourceText, mimeType
	}
	return resourceUnsupported, ""
}
