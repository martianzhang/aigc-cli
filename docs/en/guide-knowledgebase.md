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

Uses FTS5 full-text search plus semantic vector search. The embedding backend is pluggable — the built-in ONNX model (E5 multilingual), a local Ollama model, or any OpenAI-compatible `/v1/embeddings` vendor — see [Embedding backend](#embedding-backend).

## Web Search

`kb search` supports multiple web search providers with failover and weight-based routing.

### Configuration

`duckduckgo` is built-in and always available (no API key needed). Add other providers as needed:

```yaml
defaults:
  knowledgebase:
    search_provider: auto  # auto (default), quality, cheap, or provider name

web_search:
  doubao:
    type: doubao
    api_key: "xxx"
    tags: [quality]
    weight: 3          # Higher weight = more likely to be selected
    quota: 100         # Max requests per period
    period: daily      # hourly / daily / monthly

  firecrawl:
    type: firecrawl
    api_key: "xxx"
    tags: [quality]
    weight: 2
```

### Provider Selection Strategies

| Strategy | Behavior |
|----------|----------|
| `auto` (default) | Free providers first, then quality providers |
| `quality` | Only quality-tagged providers |
| `cheap` | Only free-tagged providers |
| `doubao` (specific name) | Use this provider only, with failover to others |

### Usage

```bash
# Default: auto strategy
aigc-cli kb search "query"

# Use only free providers
aigc-cli kb search "query" --provider free

# Use only quality providers
aigc-cli kb search "query" --provider quality

# Use specific provider (with failover to others)
aigc-cli kb search "query" --provider doubao

# Also search local KB
aigc-cli kb search "query" --local

# Don't save to KB
aigc-cli kb search "query" --auto-save=false

# Verbose: show which provider was used
aigc-cli kb search "query" -v
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

> The vault uses the CLI's **local encryption master key** (an age identity). It is generated automatically on the first run of any `aigc-cli` command and stored in the system keyring — `kb init` no longer creates it. On headless/CI hosts, set `AIGC_CLI_MASTER_KEY` (see the installation guide).

## Embedding backend

Semantic search embeds documents and queries with the **same** backend, chosen by `defaults.knowledgebase.embedding_provider` + `embedding_model`:

```yaml
defaults:
  knowledgebase:
    embedding_provider: ollama        # "" / local / onnx / hash, or a named provider
    embedding_model: embeddinggemma-2
```

| `embedding_provider` | Backend |
|---|---|
| *(unset)* / `local` / `onnx` | Built-in ONNX E5; falls back to the pure-Go hash embedder when ONNX is unavailable |
| `hash` | Pure-Go n-gram hash embedder (no model, low quality) |
| any name in `config.providers` | OpenAI-compatible `/v1/embeddings` — local Ollama or an online vendor |

A named provider supplies its `base_url` / `api_key` / `http_proxy`; set `embedding_model` to the vendor's model id (e.g. `embeddinggemma-2` for Ollama, `text-embedding-3-small` for OpenAI). All chunks of a document are embedded in one batched request.

> Changing the backend changes the embedding dimension, so existing vectors no longer match. `kb` prints a warning on a mismatch — run `aigc-cli kb reset` and re-add documents to rebuild the index.

## Configuration

```yaml
defaults:
  knowledgebase:
    base_dir: "~/.config/aigc-cli/knowledge"
    search_provider: duckduckgo
    auto_save: true          # Auto-save web search results to KB
    min_score: 0.8           # Minimum similarity for vector search
    embedding_provider: ollama        # "" / local / onnx / hash, or a named provider
    embedding_model: embeddinggemma-2 # embedding model id (required for a named provider)
```

## MCP / Chat Tools

When MCP Server is running, KB tools are available to AI agents:

| Tool | Description |
|---|---|
| `kb_find` | Search the knowledge base |
| `kb_search` | Web search + save to KB |
| `kb_list` | List documents |
| `kb_show` | Show document details |
