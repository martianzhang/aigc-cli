# Other Commands

## music

Generate music from a natural-language prompt (APIMart suno/flowmusic async, OpenRouter Lyria sync streaming). See [guide-music.md](guide-music.md).

## models

Query available models and pricing:

```bash
# List all models
aigc-cli models

# List configured providers (name / type / host / key status; no network request)
aigc-cli models --list-providers

# List by type
aigc-cli models --type image
aigc-cli models --type video

# OpenRouter model discovery (auth-free, no API key; every output modality)
aigc-cli models --provider openrouter --type video
aigc-cli models --provider openrouter --type speech         # TTS models + voices
aigc-cli models --provider openrouter --type transcription  # speech-to-text
aigc-cli models --provider openrouter --type embeddings
aigc-cli models --provider openrouter --type rerank
aigc-cli models --provider openrouter --type decisions
aigc-cli models --provider openrouter --type all            # everything, with output modality tags
aigc-cli models --provider openrouter --type tts            # alias: tts → speech

# Fuzzy search: only list models whose id or name contains the text
aigc-cli models --like recraft --type image   # with --type it also shows sizing
aigc-cli models --like seedream               # no --type: searches every modality

# View model pricing (OpenRouter: $/M tokens; ":free" variants are tagged)
aigc-cli models --price
aigc-cli models --provider openrouter --type speech --price

# Single model detail (context window, pricing, modalities, params)
aigc-cli models --provider openrouter openai/gpt-4o
# The global --model/-m flag works too (same as the positional argument)
aigc-cli models --provider openrouter --model openai/gpt-4o

# Print the equivalent curl without calling the API (zero cost)
aigc-cli models --model gpt-4o --dry-run
aigc-cli models --type image --dry-run
```

> **OpenRouter types**: `--type` accepts every OpenRouter output modality — `text` (default), `image`, `video`, `audio`, `speech`, `transcription`, `embeddings`, `rerank`, `decisions` and `all`. Aliases: `chat`/`llm` → `text`, `music` → `audio`, `tts` → `speech`, `stt`/`asr` → `transcription`, `embedding`/`embed` → `embeddings`, `reranking` → `rerank`, `decision` → `decisions`. A type may also be positional (`aigc-cli models -P openrouter speech`). Unknown types fail with the full list; the APIMart marketplace accepts only `image`, `video` and `chat`. Image/video discovery uses the dedicated generation endpoints (resolutions, aspect ratios, parameter ranges); every other modality and `--price` use `GET /v1/models?output_modalities=<type>`.

> **Default provider**: with no `-P/--provider`, `--api-base`, or `--api-key`, `models` uses the configured OpenRouter provider (model ids are `author/slug` and its catalogs carry the richest sizing metadata). Without an OpenRouter provider it falls back to the global `base_url`. There is no built-in API base URL: if neither a provider nor `base_url` is configured, the command fails and explains what to set.

> **`--type` / `--price` catalogs**: these flags require the provider to expose a catalog API — OpenRouter or APIMart. Any other OpenAI-compatible provider (ModelScope, relays, …) fails with a clear error instead of querying APIMart's marketplace; omit `--type` to list its `/v1/models`.

> **`--like` fuzzy matching**: case-insensitive substring match on id and display name. Without `--type` the search spans every modality (OpenRouter uses `output_modalities=all`); with `--type image` each result lists its `aspect_ratio` / `resolution` capabilities, which is the fastest way to find a model's accepted `--size` forms.

> **Single-model lookup**: `aigc-cli models <model>` or `aigc-cli models --model <model>` prints one model's details (the positional argument wins). For OpenRouter it calls the **singular** `GET /v1/model/{author}/{slug}` endpoint (not the plural `/models` used for listing) and **prints the response JSON verbatim** (pretty-printed, no field parsing). Other providers fetch the `GET /v1/models` list and filter by model id (the `GET /v1/models/{id}` endpoint is never probed — most providers do not serve it), with no extra configuration. When the model outputs images (`output_modalities` contains `image`), the CLI also queries `GET /v1/images/models` and prints its sizing capabilities (`aspect_ratio`, `resolution`, …) plus ready-to-use `--size` forms to **stderr**, leaving the JSON on stdout untouched.

> **`--dry-run`**: renders the equivalent `curl` command (API key masked) and makes no network request. It works for the plain list, single-model detail, marketplace (`--type`), pricing (`--price`), and OpenRouter discovery; marketplace pagination only renders the first page and notes the follow-up pages in a comment.

> **API key**: calling with no arguments (or with a bare model name, e.g. `aigc-cli models gpt-4o`) hits `/v1/models` and requires an API key. Without `--provider` and a global `api_key` / `OPENAI_API_KEY`, the command prints the configured providers as candidates (name / type / host / key status) and then tells you to pick one with `--provider <name>`. Run `aigc-cli models --list-providers` to print that table on its own — it makes no network request and needs no key. `--type` (type discovery, including every OpenRouter modality) and `--price` without a model name (marketplace listing with a price column) are auth-free. `--price <model>` uses APIMart's public pricing API or, for OpenRouter, the keyed `GET /v1/model/{author}/{slug}` endpoint.

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
APIMart-compatible relays), OpenRouter, DeepSeek, Moonshot (Kimi), OfoxAI,
SoMark (remaining page quota), and relays built on new-api/one-api (aibase,
agnes, openlux, …), which expose spend rather than a remaining balance. Other
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

# SoMark / OXR: remaining parse pages (paid + free today/this month)
aigc-cli balance -P somark
```

> **Probe order** for providers without a dedicated balance API: APIMart-style
> `/v1/balance` and `/v1/user/balance` first, then the OpenAI-compatible
> `/dashboard/billing/subscription` + `/dashboard/billing/usage` pair used by
> new-api/one-api relays (last 30 days of spend; the 1000000 `hard_limit_usd`
> placeholder is treated as "no limit", so no remaining balance is shown).
> "not available" appears only when every endpoint fails.

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

`aigc-cli search` searches the web or a media library (Pixabay / Pexels) and
prints **the executed vendor's raw JSON response** — no title/URL normalization.
Vendor fields such as snippets, scores, metadata and image/video download URLs
are preserved, which keeps debugging and downstream parsing straightforward.
With no third-party provider configured it falls back to the built-in free
DuckDuckGo engine — no API key required.

Media search uses `--type` to pick the endpoint and `--inject-json` for the
vendor's own filters. The query text, page size and API key stay CLI-owned and
cannot be overridden by injection. Pixabay supports `image` (all image types) /
`photo` / `illustration` / `vector` / `video`; Pexels has photos and videos only,
so it supports `image` / `video` (`image` means photos).

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
aigc-cli search --list-providers                  # registry + capability + availability
```

Media examples:

```bash
# Pixabay photos (photo / illustration / vector / image)
aigc-cli search -P pixabay "rain window night" --type photo -n 12 \
  --inject-json '{"orientation":"horizontal","min_width":1920,"safesearch":"true"}'

# Pixabay videos
aigc-cli search -P pixabay "rain window night" --type video -n 12 \
  --inject-json '{"min_width":1920}'

# Pexels videos
aigc-cli search -P pexels "rain window night" --type video -n 12 \
  --inject-json '{"orientation":"landscape","size":"large"}'
```

The raw JSON contains page URLs, preview images and download URLs; feed those
URLs to `aigc-cli fetch` to save the selected asset. Check the source license
per asset (Pixabay Content License / Pexels License).

Audio assets (SFXMint / Freesound) follow the same rule: the vendor's JSON is
passed through untouched and filters go in `--inject-json`:

```bash
# SFXMint: no API key, everything CC0, result URLs are direct WAV links
aigc-cli search -P sfxmint "door slam" -n 5
aigc-cli search -P sfxmint "ui click" -n 5 --inject-json '{"max_duration_ms":1000,"loop":false}'

# Freesound: needs api_key; use the vendor filter for CC0 only, 0-3 seconds
aigc-cli search -P freesound "door slam" -n 5 \
  --inject-json '{"filter":"license:\"Creative Commons 0\" duration:[0 TO 3]"}'

# download a result
aigc-cli fetch "https://sfxmint.com/dl/metal-door-slam-02.wav" -o sfx/
```

> Freesound's original WAV download requires OAuth2, so the CLI points result
> URLs at the public preview MP3 (`preview-hq-mp3`); download the original from
> the sound page. SFXMint result URLs are direct WAV/MP3 files that `fetch` can
> save as-is.

| Flag | Default | Description |
|---|---|---|
| `-P, --provider` | empty (→ `duckduckgo`) | Search vendor name: `duckduckgo` / `firecrawl` / `brave` / `doubao` / `deepseek` / `exa` / `wsa` / `pixabay` / `pexels` / `sfxmint` / `freesound` |
| `--type` | empty | Media endpoint: `image` / `photo` / `illustration` / `vector` / `video`; web providers ignore it |
| `-n, --limit` | `5` | Number of results to request (capped at 20) |
| `--json` | empty | Command-level JSON: file path or inline JSON (**no stdin**); fields: `query`, `provider`, `type`, `limit` |
| `--inject-json` | empty | Vendor-native fields: file path or inline JSON; merged into the POST body or appended as GET query parameters; query, page size and API key stay CLI-owned |
| `--refresh` | `false` | Ignore a provider's response cache (Pixabay caches for 24h by API requirement) |
| `--dry-run` | `false` | Print the equivalent curl for the selected provider (keys masked, injected fields included), without calling any search API |
| `--list-providers` | `false` | List configured providers with search/fetch/media capability and availability (including failure reasons) |

- **stdin carries the query**: `echo "..." | aigc-cli search` (or a no-arg
  invocation with piped input) reads it automatically; `aigc-cli search -` forces
  stdin. Multi-line input is flattened to one line. Neither `--json` nor
  `--inject-json` reads stdin.

- **`--provider` selects a search vendor, not an LLM provider.** Precedence:
  `--provider` > `--json`'s `provider` > `defaults.search.provider` > `duckduckgo`.
- Query, `--type` and limit follow the same rule: positional args / `--type` /
  `--limit` > `--json` > built-in defaults.
- `--inject-json` follows the global merge rules: objects recurse, arrays append,
  scalars overwrite. POST vendors merge it into the JSON body; GET vendors
  (Brave / Pixabay / Pexels) only accept scalars or arrays of scalars and turn
  them into query parameters — nested objects are rejected. Media queries keep
  the query text, `per_page` and API key CLI-owned.
- DuckDuckGo is an HTML scrape and rejects `--inject-json` with a clear error.
- The output is the vendor's response body: valid JSON is pretty-printed;
  non-JSON bodies are printed verbatim (useful for error pages). Media JSON
  includes preview and download URLs that can be passed to `aigc-cli fetch`.
- DuckDuckGo has no vendor JSON, so its output is `{"urls":["..."]}`.
- Exactly one provider is called per search; a provider failure is returned
  directly (no automatic rotation or retry against another vendor).
- Pixabay responses are cached for 24 hours as its API requires; do not loop
  the command for bulk scraping.
- A provider that is configured but fails construction (missing `api_key`,
  unknown `type`, …) reports `configured but unavailable` with the reason
  instead of silently disappearing from the available list;
  `--list-providers` shows the same status table.
- `--dry-run` sends no network request and prints one curl for the selected
  provider.
- Search only: no knowledge-base write. Use `aigc-cli kb search` for
  search + fetch + index. Use `aigc-cli fetch` for a standalone URL fetch.
- `-v` adds nothing for search; stdout always contains only the vendor JSON.
- All search requests honor the global proxy: `http_proxy` in config, or
  `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`.

`--json` input example:

```json
{
  "query": "rain window",
  "provider": "pexels",
  "type": "video",
  "limit": 6
}
```

`--dry-run` example (with `--inject-json`):

```bash
# dry run: no request sent
# query: golang
# strategy: manual
# provider: exa
curl -X POST https://api.exa.ai/search \
  -H "x-api-key: ...3644" \
  -H "Content-Type: application/json" \
  -d '{"numResults":4,"query":"golang","type":"neural"}'
```

## fetch

`aigc-cli fetch` downloads one URL or converts one local HTML/text file:
textual content is printed as Markdown on stdout; image/video/audio responses are
saved under the global `-o` directory and the saved path is printed. The default
is the built-in free fetcher; text is not persisted and nothing is indexed, so
redirection is enough:

```bash
aigc-cli fetch https://go.dev/doc/effective_go
aigc-cli fetch https://go.dev/doc/effective_go > effective_go.md
aigc-cli fetch ./page.html > page.md
aigc-cli fetch /tmp/report.html --max-chars 20000
aigc-cli fetch https://cdn.example/clip.mp4 -o stock/ --name rain-01
aigc-cli fetch https://cdn.example/photo.jpg -o stock/ --name still-01
aigc-cli fetch https://cdn.example/clip.mp4 -o stock/ --name rain-01.mp4 --force
aigc-cli fetch https://example.com/article --html > page.html
aigc-cli fetch https://example.com/private -H "Authorization: Bearer $TOKEN"
aigc-cli fetch https://example.com/private -b cookies.txt
aigc-cli fetch https://example.com/private -b a.txt -b b.txt -c cookies.txt
aigc-cli fetch https://example.com/private -b "session=abc; theme=dark"
aigc-cli fetch https://example.com/article --max-chars 20000
aigc-cli fetch https://example.com/app --provider deepseek
aigc-cli fetch https://example.com/article --dry-run
aigc-cli fetch --list-providers
```

| Flag | Default | Description |
|---|---|---|
| `-P, --provider` | empty = built-in | Fetch vendor name: `deepseek` / `exa` |
| `-H, --header` | none | Repeatable `Name: value`; `Name:` removes a default header (built-in fetcher only) |
| `-b, --cookie` | none | Repeatable; `name=value; ...` data or a Netscape cookie file (built-in fetcher only) |
| `-c, --cookie-jar` | none | Write cookies to a Netscape-format file after fetching (built-in fetcher only) |
| `--name` | none | File name for a media response; without an extension the detected one is appended, with an extension it is used verbatim; no path separators |
| `--force` | `false` | Overwrite an existing media file; the default is to refuse |
| `--max-chars` | `0` (no limit) | Truncate text output by rune count |
| `--html` | `false` | Print the raw HTML/response body without readability/Markdown conversion (built-in fetcher only) |
| `--dry-run` | `false` | Print the equivalent curl without sending a request |
| `--list-providers` | `false` | List configured providers with search/fetch capability and availability (including failure reasons) |
| `-v` / `--timeout` | global | `-v` prints title / final URL / content type / size; media also prints SHA256 and the saved path; `--timeout` controls the built-in fetch (use `--timeout 300` for large files) |

- Built-in fetch extracts HTML main content via readability and converts to
  Markdown; textual non-HTML bodies (plain text, JSON, …) are printed verbatim.
  Image/video/audio/PDF responses are saved under `-o` (default: current
  directory) using the response `Content-Type`; stdout contains only the saved
  path.
- Media downloads show a single-line progress on a terminal (stderr): downloaded /
  total, percent, speed and ETA. On completion — TTY or not — a
  `downloaded <size> in <time> (<speed>)` summary is printed. stdout stays the
  saved path only, so scripts remain parseable.
- Media file names: without `--name`, the URL basename is used when it carries
  a media extension; `--name rain-01` gets the detected extension appended;
  `--name rain-01.mp4` is used verbatim. The response type wins over a misleading
  URL suffix; unknown types fall back to `.bin`.
- An existing media file is refused unless `--force` is given. `--name` is a
  file name, not a path (`/` and `\` are rejected); use `-o` for the directory.
- Media auto-save is built-in-only: `--name`/`--force` cannot be combined with
  `--provider`, and do not apply to local files.
- The argument is checked as a local file first: relative/absolute paths, spaces
  and `file://` URLs work, and existing files are converted without network
  access. `--provider` / `-H` / `-b` / `-c` / `--name` / `--force` are not
  applicable to local files.
- The default User-Agent is a full browser UA, so sites are less likely to
  return a JS redirect/anti-bot stub. If the page still yields no readable text,
  fetch errors out instead of printing an empty body.
- Article pages use readability first. When readability returns only a short
  footer/navigation fragment, fetch falls back to whole-page conversion with
  scripts/styles/footers removed, preserving navigation and other useful text.
- Output strips `data:` URIs (e.g. base64 images) and `javascript:` links,
  keeping their text / normal links, so huge base64 blobs never enter the body.
- **The built-in fetcher is always the default and never reads
  `defaults.search.provider`.** Only an explicit `-P/--provider` uses a vendor,
  so a plain fetch cannot silently bill.
- `-H` is applied after the default `User-Agent` / `Accept` / `Accept-Language`
  headers and can override or remove them; CR/LF injection is rejected.
- `-b` accepts repeated inline cookie data or Netscape files; sources are merged
  and later entries win for the same domain/path/name. `#HttpOnly_`, `secure`,
  `expires`, `domain` and `path` are honored; expired cookies are ignored.
- `-H "Cookie: ..."` takes precedence over `-b`; `-H "Cookie:"` clears cookies.
- `-c` writes the merged jar (including response `Set-Cookie`) back in Netscape
  format with an atomic 0600 write — useful for login-then-fetch flows.
- `--provider` routes through a third-party fetch API (DeepSeek Fetch / Exa
  Contents), which **may bill per request**. The request runs on the vendor's
  side, so `-H` / `-b` / `-c` / `--name` / `--force` cannot be combined with
  `--provider`.
- `--html` prints the raw HTML (or the raw text body for non-HTML textual
  responses / local text files), skipping readability/Markdown; it only applies
  to text responses (media is still saved) and cannot be combined with
  `--provider`.
- If the selected vendor is search-only (e.g. `brave` / `duckduckgo`), fetch
  falls back to the built-in fetcher and prints a note on stderr; `-H` / `-b` /
  `-c` still work. An unknown provider name is still an error.
- Fetch only: nothing is written to the knowledge base. Use `aigc-cli kb fetch`
  to fetch + index; use `aigc-cli search` / `kb search` to search.
- `--output-json` is not implemented yet.
- Both built-in and third-party fetch honor the global proxy: `http_proxy` in
  config, or `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`.

`--dry-run` example:

```bash
# dry run: no request sent
# provider: built-in
curl -X GET https://example.com/article \
  -H "User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36" \
  -H "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.5" \
  -H "Accept-Language: zh-CN,zh;q=0.9,en;q=0.8"
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

`search` / `fetch` also support dynamic `--provider` completion: candidates come
from the current `web_search.*` config; `fetch` only suggests fetch-capable
providers.

## Error hints

Usage errors get a short, command-scoped explanation instead of a full usage dump:

```bash
$ aigc-cli image --modle "a cat"
Error: unknown flag: --modle
Did you mean --mode, --model?
Run 'aigc-cli image --help' for usage and examples.

$ aigc-cli image --n abc
Error: invalid argument "abc" for "--n" flag: strconv.ParseInt: parsing "abc": invalid syntax
Invalid value "abc" for --n: expected an integer, such as 1.
Run 'aigc-cli image --help' for usage and examples.
```

- Misspelled long flags suggest up to three close names (adjacent transpositions count as one edit); an unknown shorthand lists the shorthands the command actually defines.
- Wrong-typed values show a concrete example for integers, numbers, booleans, durations, and repeatable flags.
- Hints always point at the failing command's own `--help`, never the root command.
- Runtime errors (API, network, task failures) stay concise and do not gain usage hints.
- Downloaded and generated results (images, videos, audio, base64-decoded input) are written to a temporary file in the destination directory and renamed into place. If a download or write fails, an existing file at that path is left unchanged.

## Global Flags

| Flag | Description |
|---|---|
| `--dry-run` | Print request params and equivalent curl, no API call |
| `--print-config` | Print effective config with source annotations (secrets masked: API keys show only the last 4 chars, URL credentials/keys become `REDACTED`) |
| `-v` / `--verbose` | Show detailed output: full JSON, token usage, timing, cost |
| `--json` | Pass request as JSON (file, string, or stdin) |
| `--inject-json` | Merge a JSON object on top of the generated body (objects recurse, arrays append, scalars overwrite); for vendor-only request fields, takes precedence over `--json` |
| `--preview` | Open system preview after generation |
| `--save-prompt` | Save the prompt as `<kind>_<task_id>.md` next to the output (`image` / `video`); works with every provider |
| `--http-proxy` | Specify HTTP proxy |
| `--no-proxy` | Ignore `http_proxy` config and `HTTP_PROXY` / `HTTPS_PROXY` for this run (direct connection) |
| `--zdr` | Request zero data retention where supported. OpenRouter chat-completions endpoints (`chat`, `music`) honor it via `provider.zdr` + `data_collection: deny`; every other provider/endpoint is a silent no-op |
| `--config` | Path to config file (must exist; a missing explicit path is an error. Omit it to fall back to env vars and defaults) |
| `--api-key` | API key (overrides config/env) |
| `--api-base` | API base URL (overrides config/env) |
| `--model` / `-m` | Model name |
| `--provider` / `-P` | Named provider reference |
| `--output` / `-o` | Output directory (created automatically when missing) |
| `--timeout` | HTTP request timeout in seconds; also caps async task polling (per-media defaults live in `defaults.<command>.timeout`) |
