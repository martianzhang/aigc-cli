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

> Just want web results without fetching or indexing? Use `aigc-cli search "query"` (free DuckDuckGo fallback by default). `kb search` additionally fetches page bodies, chunks and stores them.
>
> Just want one URL as Markdown without indexing? Use `aigc-cli fetch <url>`; `kb fetch` fetches and stores it in the knowledge base.

### Configuration

`duckduckgo` is built-in and always available (no API key needed). The default vendor lives in `defaults.search.provider`. Add other providers as needed:

```yaml
defaults:
  search:
    provider: deepseek  # a vendor name; empty falls back to duckduckgo

web_search:
  doubao:
    type: doubao
    api_key: "xxx"

  firecrawl:
    type: firecrawl
    api_key: "xxx"

  deepseek:
    type: deepseek
    api_key: "sk-xxx"
  tencent:
    type: wsa
    api_key: "sk-xxx"   # Tencent Yuanbao Search API KEY (SearchPro)    # DeepSeek Search (in-house engine, $0.75/1000)

  exa:
    type: exa
    api_key: "exa-xxx"   # Exa neural search (Search + Contents/fetch, $10 free credit/month)
```

> **Tencent Yuanbao Search (wsa)**: API KEY mode `POST https://api.wsa.cloud.tencent.com/SearchPro`, authenticated with `Authorization: Bearer <API KEY>`. Results are JSON strings in `Pages` (title/url/passage/date). The console currently offers a free trial; check the [console](https://console.cloud.tencent.com/wsapi/index) for version and quota. Use `type: wsa`; the provider name is arbitrary (the example uses `tencent`).
>
> **DeepSeek search**: `POST https://api.deepseek.com/v1/tools/search`, billed per successful request ($0.75/1000 ≈ ¥5/1000 — independent of result count). Reuse the same key as `providers.deepseek`.
> Note: only **server-side tool calls in DeepSeek Harness / Claude Code / Codex** are free until 2026-11-01; this CLI calling the endpoint directly is **billed immediately**.

### Provider Selection

Exactly one provider is used per search: `--provider` or
`defaults.search.provider`, falling back to `duckduckgo`. There is no automatic
rotation, strategy, or failover.

### Usage

```bash
# Default: defaults.search.provider, or duckduckgo
aigc-cli kb search "query"

# Use a specific provider
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
    embedding_model: embeddinggemma-2:270m
```

| `embedding_provider` | Backend |
|---|---|
| *(unset)* / `local` / `onnx` | Built-in ONNX E5; falls back to the pure-Go hash embedder when ONNX is unavailable |
| `hash` | Pure-Go n-gram hash embedder (no model, low quality) |
| any name in `config.providers` | OpenAI-compatible `/v1/embeddings` — local Ollama or an online vendor |

A named provider supplies its `base_url` / `api_key` / `http_proxy`; set `embedding_model` to the vendor's model id (e.g. the **text-only** `embeddinggemma-2:270m` / `:570m` for Ollama — `:latest` / `:740m` are multimodal and unnecessary for text; `text-embedding-3-small` for OpenAI). All chunks of a document are embedded in one batched request.

> Changing the backend changes the embedding dimension, so existing vectors no longer match. `kb` prints a warning on a mismatch — run `aigc-cli kb reset` and re-add documents to rebuild the index.

## Configuration

```yaml
defaults:
  search:
    provider: duckduckgo
  knowledgebase:
    base_dir: "~/.config/aigc-cli/knowledge"
    auto_save: true          # Auto-save web search results to KB
    min_score: 0.8           # Minimum similarity for vector search (default 0.8; 0 disables filtering)
    embedding_provider: ollama        # "" / local / onnx / hash, or a named provider
    embedding_model: embeddinggemma-2:270m # embedding model id (required for a named provider)
```

## MCP / Chat Tools

When MCP Server is running, KB tools are available to AI agents:

| Tool | Description |
|---|---|
| `kb_find` | Search the knowledge base |
| `kb_search` | Web search + save to KB |
| `kb_list` | List documents |
| `kb_show` | Show document details |
