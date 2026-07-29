# Knowledge Base

Use `aigc-cli knowledgebase` (alias `kb`) to manage a local knowledge base with full-text and semantic search.

## Commands

```
aigc-cli kb
├── init          Initialize the knowledge base
├── add <path>    Add a local file or directory
├── fetch <url>   Fetch a URL and add it
├── map <url>     Discover URLs from a page and batch fetch
├── find <query>  Search the knowledge base
├── search <q>    Web search + save to knowledge base
├── list          List all documents
├── show <id>     Show document details
├── rm <id>       Remove a document
├── prune         Remove duplicates and dead URLs
├── reset         Delete all data and reinitialize
├── index         Re-index all documents
└── vault         Encrypted document management
```

## Search

```bash
# Search local KB
aigc-cli kb find "machine learning"

# Web search + save to KB
aigc-cli kb search "latest AI news"
```

Uses FTS5 full-text search and ONNX semantic search (E5 multilingual embedding).

## Web Search

`kb search` supports multiple web search providers configured via `web_search` in config.yaml:

```yaml
web_search:
  duckduckgo:
    type: duckduckgo    # Zero config, free
  brave:
    type: brave
    api_key: "BSA-xxx"
  firecrawl:
    type: firecrawl
    api_key: "fc-xxx"
  doubao:
    type: doubao
    api_key: "your-doubao-api-key"   # Volcengine Doubao search
```

```bash
# Use default provider
aigc-cli kb search "query"

# Use specific provider
aigc-cli kb search "query" --provider doubao

# Don't save to KB
aigc-cli kb search "query" --auto-save=false
```

## Document Organization

Documents are stored as markdown files:

```
~/.config/aigc-cli/knowledge/
├── knowledge.db     # SQLite (FTS5 + vectors + metadata)
└── docs/
    ├── global/      # Project-less documents
    └── <project>/   # Git project-scoped documents
```

Files are named `<date>_<shortID>-<slug>.md` for easy browsing.

## List

```bash
# List all documents in current project
aigc-cli kb list

# List all projects
aigc-cli kb list --all

# Paginate
aigc-cli kb list --limit 10 --offset 20
```

## Vault

Encrypted document storage using age encryption:

```bash
aigc-cli kb vault lock     # Encrypt a file
aigc-cli kb vault unlock   # Decrypt and read
aigc-cli kb list --vault   # List vault documents
```

## Configuration

```yaml
defaults:
  knowledgebase:
    base_dir: "~/.config/aigc-cli/knowledge"
    search_provider: duckduckgo
    auto_save: true          # Auto-save web search results to KB
    min_score: 0.8           # Minimum similarity for vector search
```

## MCP / Chat Tools

When MCP Server is running, KB tools are available to AI agents:

| Tool | Description |
|---|---|
| `kb_find` | Search the knowledge base |
| `kb_search` | Web search + save to KB |
| `kb_list` | List documents |
| `kb_show` | Show document details |
