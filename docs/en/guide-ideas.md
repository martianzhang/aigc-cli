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

# Search a single online source
aigc-cli ideas "cyberpunk city" --source aipromptslibrary
aigc-cli ideas "portrait" --source prompts.chat
aigc-cli ideas "portrait" --source openart
aigc-cli ideas "cyberpunk city" --source civitai

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
| `civitai` | [civitai.com](https://civitai.com) image prompts — the keyword is resolved through Civitai's model search, then the prompts are read from those models' image metadata (Civitai exposes no prompt-search endpoint). Results are NSFW-filtered (`nsfw=None`). |

Notes:

- Pass one value, a comma-separated list, or repeat the flag; names are case-insensitive and duplicates are ignored.
- When `ideas.json` is absent, the default `all` silently searches the online sources only. Passing `--source local` explicitly without `ideas.json` is an error — run `aigc-cli ideas init` first.
- Online sources need network access; the configured `http_proxy` (config.yaml, `HTTP_PROXY`, or `--http-proxy`) is respected.
- If an online source fails, a warning is printed to stderr and results from the other sources are still returned. If every source comes back empty, the command prints `没有找到匹配的提示词。` (no matching prompts found).
- Results from every source are fused with reciprocal rank fusion, so local and online hits are interleaved by relevance.

## Parameters

| Flag | Description |
|---|---|
| `--source` | Sources to search: `all` (default), `local`, `aipromptslibrary`, `prompts.chat`, `openart`, `civitai` |
| `--limit`, `-l` | Number of results to show (default 8) |
| `--random` | Shuffle matched results randomly |
| `--json` | Output JSON instead of markdown |
| `--save` | Download reference images to a local directory |
| `--find-image` | Search by image filename (local dataset only) |
| `--preview` | Open saved images with the system default viewer (implies `--save`) |

## Output

Markdown by default, with each result containing a title, reference images, the full prompt text, and metadata. With `--json`, the fused result list is emitted as a JSON object.

```bash
# Markdown (default), easy to redirect to a file
aigc-cli ideas "cat" > my-ideas.md

# JSON output, easy to filter with jq
aigc-cli ideas "portrait" --json | jq '.results[].prompt'

# Search -> extract prompt -> generate an image
aigc-cli ideas "cat" --json \
  | jq -r '.results[0].prompt' \
  | aigc-cli image --model gpt-image-2 --prompt -
```
