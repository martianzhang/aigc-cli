package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/yaml.v3"

	"github.com/martianzhang/aigc-cli/internal/config"
	"github.com/martianzhang/aigc-cli/internal/service"
	"github.com/martianzhang/aigc-cli/internal/types"
)

// Resource URIs exposed by the MCP server. All are read-only views over the
// configured providers, the effective config, and the output directory.
const (
	providersResourceURI = "aigc://providers"
	configResourceURI    = "aigc://config"
	outputResourceURI    = "aigc://output"
	outputTemplateURI    = "aigc://output/{filename}"

	// maxResourceEntries caps how many output-directory entries a listing shows.
	maxResourceEntries = 200
)

// registerResources registers the read-only MCP resources: provider names,
// masked effective config, an output-directory listing, and a per-file
// template constrained to the output directory.
func registerResources(s *server.MCPServer, cfg *Config) {
	s.AddResource(mcp.NewResource(providersResourceURI, "Configured providers",
		mcp.WithResourceTitle("Configured providers"),
		mcp.WithResourceDescription("Names of the providers configured in config.providers, with their host. Use these names for the `provider` tool argument. API keys are never included."),
		mcp.WithMIMEType("application/json"),
	), providersResourceHandler(cfg))

	s.AddResource(mcp.NewResource(configResourceURI, "Effective config",
		mcp.WithResourceTitle("Effective config"),
		mcp.WithResourceDescription("Effective aigc-cli configuration with all secrets masked (same guarantee as --print-config)."),
		mcp.WithMIMEType("application/yaml"),
	), configResourceHandler())

	s.AddResource(mcp.NewResource(outputResourceURI, "Output directory",
		mcp.WithResourceTitle("Output directory"),
		mcp.WithResourceDescription("Top-level listing (name, size, modified time) of the configured output directory. Non-recursive."),
		mcp.WithMIMEType("text/plain"),
	), outputResourceHandler(cfg))

	s.AddResourceTemplate(mcp.NewResourceTemplate(outputTemplateURI, "Output file",
		mcp.WithTemplateTitle("Output file"),
		mcp.WithTemplateDescription("Read one file from the configured output directory by file name. Images and audio are returned as base64 blobs; .txt/.md/.json/.yaml/.yml/.csv/.log as text."),
	), outputFileHandler(cfg))
}

// providerSummary is the credential-free view of a named provider.
type providerSummary struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	Host string `json:"host,omitempty"`
}

// providersResourceHandler lists configured provider names, sorted, together
// with the host part of each base URL (never credentials or query parameters).
func providersResourceHandler(cfg *Config) server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		names := make([]string, 0, len(cfg.Providers))
		for name := range cfg.Providers {
			names = append(names, name)
		}
		sort.Strings(names)

		summaries := make([]providerSummary, 0, len(names))
		for _, name := range names {
			summaries = append(summaries, summarizeProvider(name, cfg.Providers[name]))
		}
		data, err := json.MarshalIndent(map[string]any{"providers": summaries}, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("marshal providers: %w", err)
		}
		return textContents(req, "application/json", string(data)), nil
	}
}

// summarizeProvider projects a named provider onto its safe, credential-free fields.
func summarizeProvider(name string, p *types.NamedProvider) providerSummary {
	sum := providerSummary{Name: name}
	if p == nil {
		return sum
	}
	sum.Type = string(p.Type)
	sum.Host = urlHost(p.BaseURL)
	return sum
}

// urlHost extracts only the host name from a URL, dropping userinfo, port,
// path, and query parameters so no credential can leak.
func urlHost(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// configResourceHandler mirrors the get_config tool: load, mask, marshal.
func configResourceHandler() server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		cfg, err := config.Load("")
		if err != nil {
			return nil, fmt.Errorf("load config: %w", err)
		}
		if cfg == nil {
			cfg = &types.Config{}
		}
		cfg = service.MaskConfigSecrets(cfg)
		b, err := yaml.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("marshal config: %w", err)
		}
		return textContents(req, "application/yaml", string(b)), nil
	}
}

// outputResourceHandler lists the configured output directory.
func outputResourceHandler(cfg *Config) server.ResourceHandlerFunc {
	return func(_ context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
		text, err := outputListing(cfg.Output)
		if err != nil {
			return nil, err
		}
		return textContents(req, "text/plain", text), nil
	}
}

// outputListing renders the top level of dir (non-recursive) as one line per
// regular file: name, size, and modified time. Directories and symlinks are
// skipped so the listing cannot point outside dir. The entry count is capped.
func outputListing(dir string) (string, error) {
	if dir == "" {
		return "no output directory configured", nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("output directory does not exist: %s", dir), nil
		}
		return "", fmt.Errorf("read output directory: %w", err)
	}

	files := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			files = append(files, entry)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })

	var b strings.Builder
	fmt.Fprintf(&b, "Output directory: %s\n", dir)
	shown := len(files)
	if shown > maxResourceEntries {
		shown = maxResourceEntries
	}
	for _, entry := range files[:shown] {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "- %s (%d bytes, %s)\n", entry.Name(), info.Size(), info.ModTime().Format("2006-01-02 15:04:05"))
	}
	if len(files) > shown {
		fmt.Fprintf(&b, "... and %d more (listing capped at %d entries)\n", len(files)-shown, maxResourceEntries)
	}
	return b.String(), nil
}

// textContents wraps text in a single TextResourceContents block.
func textContents(req mcp.ReadResourceRequest, mimeType, text string) []mcp.ResourceContents {
	return []mcp.ResourceContents{mcp.TextResourceContents{
		URI:      req.Params.URI,
		MIMEType: mimeType,
		Text:     text,
	}}
}
