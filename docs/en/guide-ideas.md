# Prompt Ideas

Use `aigc-cli ideas` (alias `idea`) to search prompt idea libraries: the built-in local dataset, four online prompt libraries, or all of them fused into one ranked list.

Local search is fully offline (BM25, CJK-aware, n-gram, RRF). Online sources are keyless JSON APIs, so no API key is required — but they do need network access.

## Usage

```bash
# Random prompt idea (local dataset)
aigc-cli ideas

# Search by keyword — local + online by default
aigc-cli ideas "cat"
aigc-cli ideas "cyberpunk city"

# Search in Chinese
aigc-cli ideas "猫"

# Print only the prompt text (ready to pipe into aigc-cli image)
aigc-cli ideas --random --limit 1 --prompt-only

# Fetch a previously seen idea by ID
aigc-cli ideas --id 1a2b3c4d5e6f --prompt-only

# An explicit --limit is honored without keywords (only a bare `ideas` defaults to 1)
aigc-cli ideas --limit 2

# Search a single online source
aigc-cli ideas "cyberpunk city" --source aipromptslibrary
aigc-cli ideas "portrait" --source prompts.chat
aigc-cli ideas "portrait" --source openart
aigc-cli ideas "cyberpunk city" --source civitai
aigc-cli ideas "cyberpunk city" --source youmind

# Combine sources (comma-separated or repeatable)
aigc-cli ideas "portrait" --source local,openart
aigc-cli ideas "portrait" --source prompts.chat --source openart
```

The local idea library contains 10,000+ prompts covering various styles, subjects, and artistic techniques. Download it with `aigc-cli ideas init` (stored at `~/.config/aigc-cli/ideas/ideas.json`).

## Sources (`--source`)

| Value | Description |
|---|---|
| `all` | Local dataset (when `ideas.json` exists) plus all online sources. **Default.** |
| `local` | Local `ideas.json` dataset only — fully offline BM25 search. |
| `aipromptslibrary` | [aipromptslibrary.sh](https://aipromptslibrary.sh) image-generation prompts. |
| `prompts.chat` | [prompts.chat](https://prompts.chat) community prompt library. |
| `openart` | [openart.ai](https://openart.ai) community prompts — **experimental**: the search endpoint is undocumented and has no canonical per-item URL, so results carry no source link. |
| `youmind` | [youmind.com/prompts](https://youmind.com/prompts) prompt library — calls the site's public JSON API and queries image, video and webpage prompts concurrently, de-duplicating by source URL. No API key; returns the full prompt text, author, reference media and detail link. |
| `civitai` | [civitai.com](https://civitai.com) image prompts — the keyword is resolved through Civitai's model search, then the prompts are read from those models' image metadata (Civitai exposes no prompt-search endpoint). Results are NSFW-filtered (`nsfw=None`). |

Notes:

- Pass one value, a comma-separated list, or repeat the flag; names are case-insensitive and duplicates are ignored.
- When `ideas.json` is absent, the default `all` silently searches the online sources only. Passing `--source local` explicitly without `ideas.json` is an error — run `aigc-cli ideas init` first.
- Online sources need network access; the configured `http_proxy` (config.yaml, `HTTP_PROXY`, or `--http-proxy`) is respected.
- If an online source fails, a warning is printed to stderr and results from the other sources are still returned. If every source comes back empty, the command prints `没有找到匹配的提示词。` (no matching prompts found).
- Results from every source are fused with reciprocal rank fusion, so local and online hits are interleaved by relevance.

## Semantic search (embedding)

By default search is BM25 keyword-only. Set `embedding_provider` + `embedding_model` in the top-level `ideas` config to add semantic recall fused with the keyword results via RRF — matches surface even when the wording differs (e.g. "calm nature scenery in the early morning" finds `serene mountain lake at sunrise`).

```yaml
defaults:
  ideas:
    embedding_provider: ollama            # "" / local / onnx / hash, or a name in config.providers
    embedding_model: embeddinggemma-2:270m
    embedding_timeout: 180                # per-request embedding timeout in seconds (default 180)
    embedding_max_runes: 256              # truncate embedded text (default 256; smaller = faster; <=0 = no truncation)
    semantic_top_k: 200                   # entries the semantic list contributes to RRF (default 200; <=0 = all)
```

- Text embeddings need only a small **text-only** model: `embeddinggemma-2:270m` (378MB) or `:570m` (990MB); `:latest` / `:740m` are multimodal and unnecessary for text.
- The first search precomputes vectors for the whole dataset and caches them next to `ideas.json` (`ideas_emb_<model>_<dim>.bin`); later searches are instant. Changing the model or dataset rebuilds it.
- With no `embedding_provider`, search stays keyword-only (fully backward compatible).

## Parameters

| Flag | Description |
|---|---|
| `--source` | Sources to search: `all` (default), `local`, `aipromptslibrary`, `prompts.chat`, `openart`, `civitai`, `youmind` |
| `--limit`, `-l` | Number of results to show (default 8; a bare `ideas` with no keywords and no explicit `--limit` returns 1 random idea) |
| `--random` | Shuffle matched results randomly |
| `--id` | Fetch one idea by its ID (local dataset only; IDs are printed with each result) |
| `--prompt-only` | Print only the prompt text, for piping into `aigc-cli image -p` |
| `--save` | Download reference images to a local directory |
| `--find-image` | Search by image filename (local dataset only) |
| `--preview` | Open saved images with the system default viewer (implies `--save`) |

## Output

Markdown by default, with each result containing a title, an **idea ID**, reference images, the full prompt text, and metadata.

```bash
# Markdown (default), easy to redirect to a file
aigc-cli ideas "cat" > my-ideas.md
```

### Reuse a prompt by ID

Each result prints a stable 12-character hex `ID`, derived from the entry content (`source_url` + `prompt`). The ID survives dataset reordering and upgrades as long as the content is unchanged.

```bash
# 1. Look at a random idea and note the printed ID
aigc-cli ideas

# 2. Fetch the same idea later (--prompt-only prints just the prompt text)
aigc-cli ideas --id 1a2b3c4d5e6f --prompt-only

# 3. Generate an image from it directly
aigc-cli image -p "$(aigc-cli ideas --id 1a2b3c4d5e6f --prompt-only)"

# Or do "random idea -> image" in one line
aigc-cli image -p "$(aigc-cli ideas --random --limit 1 --prompt-only)"
```

Notes:

- `--prompt-only` prints only the prompt body — no title, images, metadata or Markdown markup — so it is safe for command substitution and pipes.
- `--id` searches the **local `ideas.json`** only. Online-source results are fetched on demand and not persisted, so they cannot be fetched again by ID.
- ID matching is case-insensitive.
