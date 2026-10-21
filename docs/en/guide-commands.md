# Other Commands

## music

Generate music from a natural-language prompt (APIMart suno/flowmusic async, OpenRouter Lyria sync streaming). See [guide-music.md](guide-music.md).

## models

Query available models and pricing:

```bash
# List all models
aigc-cli models

# List by type
aigc-cli models --type image
aigc-cli models --type video

# OpenRouter model discovery (auth-free; GET /v1/images/models or /v1/videos/models)
aigc-cli models --provider openrouter --type video

# View model pricing
aigc-cli models --price

# Single model detail (context window, pricing, modalities, params)
aigc-cli models --provider openrouter openai/gpt-4o
# The global --model/-m flag works too (same as the positional argument)
aigc-cli models --provider openrouter --model openai/gpt-4o

# Print the equivalent curl without calling the API (zero cost)
aigc-cli models --model gpt-4o --dry-run
aigc-cli models --type image --dry-run
```

> **Single-model lookup**: `aigc-cli models <model>` or `aigc-cli models --model <model>` prints one model's details (the positional argument wins). For OpenRouter it calls the **singular** `GET /v1/model/{author}/{slug}` endpoint (not the plural `/models` used for listing) and **prints the response JSON verbatim** (pretty-printed, no field parsing). Other providers fetch the `GET /v1/models` list and filter by model id (the `GET /v1/models/{id}` endpoint is never probed — most providers do not serve it), with no extra configuration.

> **`--dry-run`**: renders the equivalent `curl` command (API key masked) and makes no network request. It works for the plain list, single-model detail, marketplace (`--type`), pricing (`--price`), and OpenRouter discovery; marketplace pagination only renders the first page and notes the follow-up pages in a comment.

> **API key**: calling with no arguments (or with a bare model name, e.g. `aigc-cli models gpt-4o`) hits `/v1/models` and requires an API key. Without `--provider` and a global `api_key` / `OPENAI_API_KEY`, the command fails fast with a clear error listing the configured providers; use `--provider <name>` to pick one, or supply `--api-key`. `--type` and `--price` (marketplace and pricing) are auth-free — no key needed.

## task

Query async task status (APIMart compatible):

```bash
# Use the globally configured provider
aigc-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3

# Query through a named provider's account and base URL
aigc-cli task --provider apimart task_01KV4KD9FBH3AZ4DE18A7Y17S3
```

`--provider <name>` selects which provider's account and base URL are queried, and the task must belong to that provider. Without it, a missing key fails fast with the list of configured providers that have keys.

## balance

Query account balance across providers that expose a balance API: APIMart (and
APIMart-compatible relays), OpenRouter, DeepSeek, and Moonshot (Kimi). Other
providers have no public balance API — check their web console.

**By default every balance endpoint a provider exposes is queried and shown**
(e.g. APIMart's key quota + account balance, OpenRouter's key usage + account
credits). The `token` / `user` subcommands narrow the output to one scope.

```bash
# Query every configured provider that has an API key (all endpoints)
aigc-cli balance

# Query one specific provider
aigc-cli balance --provider openrouter

# Narrow to the API-key scope / the account scope
aigc-cli balance token
aigc-cli balance user
```

> **API key**: without `--provider`, every configured provider that has an API key is queried (local providers such as Ollama are exempt). An explicitly selected non-local provider without a key fails fast with a clear error; use `--provider <name>` or configure its key.

> **OpenRouter**: account credits (`GET /credits`, matching the console) require a management key; a regular inference key only yields key-scoped usage/limits (`GET /key`).

> **SiliconFlow**: its `/user/info` balance endpoint was retired on 2026-08-14 with no replacement yet, so it reports "not available".

## config

Read and edit `config.yaml` without opening an editor. Keys are dot paths that
match the YAML structure:

```bash
# Print one value (secrets are masked)
aigc-cli config get defaults.image.model

# Print a whole section (nested secrets are masked too)
aigc-cli config get providers

# Set one value: atomic write, previous content backed up to config.yaml.bak
aigc-cli config set defaults.image.model gpt-image-2

# Print the effective config with secrets masked
aigc-cli config list
```

Notes:

- The file is resolved like every other command: `--config <path>` first, otherwise `~/.config/aigc-cli/config.yaml`. An explicit `--config` path must exist: any command fails with `config file not found` instead of silently ignoring it. When `--config` is omitted, a missing file is fine and the CLI falls back to env vars (`OPENAI_API_KEY` / `OPENAI_BASE_URL`) and code defaults. `get`/`set` fail with a clear error and never create the file.
- `set` replaces one leaf only. If a parent section of the dot path does not exist, the command fails (`set defaults.chat.allow_tool_override: section not found`) instead of creating sections.
- The existing YAML type of a key is kept: an int/bool/float stays that type and a value that cannot be parsed as it is rejected; existing strings stay strings. New keys inside an existing section become plain strings unless the value is a plain integer or `true`/`false`.
- Writing is atomic: the previous file is copied to `<path>.bak`, the new content goes to `<path>.tmp.<pid>`, then that file is renamed over the target. Comments, key order and scalar styles survive the round-trip; blank lines and unrelated spacing may be normalized.
- `api_key` / `api_secret` (including under `providers.*` and `web_search.*`) are encrypted with the local master key before they are written (`enc:v1:...`). In addition, any `aigc-cli` command scans `config.yaml` at startup and encrypts any plaintext secret it finds. Encryption writes no plaintext `.bak`, and other fields are untouched.
- Secrets are never printed in full: `api_key` shows only the last 4 chars (`...abcd`), and credentials inside `base_url` / `http_proxy` become `REDACTED` — the same masking as `--print-config`.
- `api_key` and `base_url` (global or under `providers.*`) require `--force`, because they control where credentials are sent:

```bash
aigc-cli config set api_key sk-xxx
# Error: refusing to set api_key without --force: api_key/base_url hold credentials or endpoint overrides

aigc-cli config set api_key sk-xxx --force
```

- `config list` works without a config file: it prints the code defaults with a `# config file not found` comment.

## dry-run

See what API request would be sent without actually calling:

```bash
aigc-cli image --prompt "a cat" --dry-run
```

Prints the HTTP method, URL, headers, and JSON body.

## search

`aigc-cli search` searches the web and prints **the executed vendor's raw JSON
response** — no title/URL normalization. Vendor fields such as snippets, scores
and metadata are preserved, which keeps debugging and downstream parsing
straightforward. With no third-party provider configured it falls back to the
built-in free DuckDuckGo engine — no API key required.

```bash
aigc-cli search "golang context timeout"
aigc-cli search golang context timeout            # multiple args form one query
aigc-cli search "vector database" -n 10           # request 10 results (default 5, max 20)
aigc-cli search "vector database" -P deepseek     # pick a search vendor
aigc-cli search "vector database" -P free         # free engines only
echo "golang context timeout" | aigc-cli search  # read the query from stdin
aigc-cli search - < query.txt                     # explicit stdin
aigc-cli search --json query.json                 # command-level JSON (file)
aigc-cli search --json '{"query":"golang context","provider":"exa","limit":3}'
aigc-cli search "golang" --inject-json '{"type":"neural"}'
aigc-cli search "vector database" --dry-run       # print equivalent curl, no request
```

| Flag | Default | Description |
|---|---|---|
| `-P, --provider` | empty (→ auto) | Search vendor: `duckduckgo` / `firecrawl` / `brave` / `doubao` / `deepseek` / `exa`, or a strategy: `auto` / `free` / `quality` |
| `-n, --limit` | `5` | Number of results to request (capped at 20) |
| `--json` | empty | Command-level JSON: file path or inline JSON (**no stdin**); fields: `query`, `provider`, `limit` |
| `--inject-json` | empty | Vendor-native fields: file path or inline JSON; merged into the POST body or appended as GET query parameters |
| `--dry-run` | `false` | Print the equivalent curl for each candidate provider (keys masked, injected fields included), without calling any search API |

- **stdin carries the query**: `echo "..." | aigc-cli search` (or a no-arg
  invocation with piped input) reads it automatically; `aigc-cli search -` forces
  stdin. Multi-line input is flattened to one line. Neither `--json` nor
  `--inject-json` reads stdin.

- **`--provider` selects a search vendor, not an LLM provider.** Precedence:
  `--provider` > `--json`'s `provider` > `defaults.search.provider` > `auto`.
- Query and limit follow the same rule: positional args / `--limit` > `--json` >
  built-in defaults.
- `--inject-json` follows the global merge rules: objects recurse, arrays append,
  scalars overwrite. POST vendors merge it into the JSON body; GET vendors
  (Brave) only accept scalars or arrays of scalars and turn them into query
  parameters — nested objects are rejected.
- DuckDuckGo is an HTML scrape and rejects `--inject-json` with a clear error.
- The output is the vendor's response body: valid JSON is pretty-printed;
  non-JSON bodies are printed verbatim (useful for error pages).
- DuckDuckGo has no vendor JSON, so its output is `{"urls":["..."]}`.
- Under `auto`, providers tagged `free` come first in the candidate list, then the
  router picks by `weight`; use `-P free` to force free engines or `-P <name>` to
  pin one vendor.
- `--dry-run` sends no network request. Under `auto` it prints one curl per
  candidate; only one of them will actually run.
- Search only: no knowledge-base write. Use `aigc-cli kb search` for
  search + fetch + index. A standalone `aigc-cli fetch` is still planned.
- `-v` adds nothing for search; stdout always contains only the vendor JSON.

`--json` input example:

```json
{
  "query": "golang context",
  "provider": "exa",
  "limit": 3
}
```

`--dry-run` example (with `--inject-json`):

```bash
# dry run: no request sent
# query: golang
# strategy: manual
# provider: exa (candidate 1/1)
curl -X POST https://api.exa.ai/search \
  -H "x-api-key: ...3644" \
  -H "Content-Type: application/json" \
  -d '{"numResults":4,"query":"golang","type":"neural"}'
```

## completion

Generate shell completion scripts:

```bash
# Bash
aigc-cli completion bash > /etc/bash_completion.d/aigc-cli

# Zsh
aigc-cli completion zsh > /usr/local/share/zsh/site-functions/_aigc-cli

# Fish
aigc-cli completion fish > ~/.config/fish/completions/aigc-cli.fish

# PowerShell
aigc-cli completion powershell > aigc-cli.ps1
```

## Global Flags

| Flag | Description |
|---|---|
| `--dry-run` | Print request params and equivalent curl, no API call |
| `--print-config` | Print effective config with source annotations (secrets masked: API keys show only the last 4 chars, URL credentials/keys become `REDACTED`) |
| `-v` / `--verbose` | Show detailed output: full JSON, token usage, timing, cost |
| `--json` | Pass request as JSON (file, string, or stdin) |
| `--inject-json` | Merge a JSON object on top of the generated body (objects recurse, arrays append, scalars overwrite); for vendor-only request fields, takes precedence over `--json` |
| `--preview` | Open system preview after generation |
| `--save-prompt` | Save prompt as .md file |
| `--http-proxy` | Specify HTTP proxy |
| `--zdr` | Request zero data retention where supported. OpenRouter chat-completions endpoints (`chat`, `music`) honor it via `provider.zdr` + `data_collection: deny`; every other provider/endpoint is a silent no-op |
| `--config` | Path to config file (must exist; a missing explicit path is an error. Omit it to fall back to env vars and defaults) |
| `--api-key` | API key (overrides config/env) |
| `--api-base` | API base URL (overrides config/env) |
| `--model` / `-m` | Model name |
| `--provider` / `-P` | Named provider reference |
| `--output` / `-o` | Output directory |
| `--timeout` | HTTP request timeout in seconds; also caps async task polling (per-media defaults live in `defaults.<command>.timeout`) |
