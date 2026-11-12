package mcp

// ToolCatalogEntry is the public metadata used to generate the tool reference.
type ToolCatalogEntry struct {
	Name        string
	Description string
}

// ToolCatalog returns the registered tool names and short descriptions in
// registry order. Callers (docs generators) must not mutate the returned slice.
func ToolCatalog() []ToolCatalogEntry {
	out := make([]ToolCatalogEntry, 0, len(toolRegistry))
	for _, info := range toolRegistry {
		out = append(out, ToolCatalogEntry{Name: info.name, Description: info.desc})
	}
	return out
}
