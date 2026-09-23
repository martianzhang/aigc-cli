# MCP Integration

aigc-cli provides a built-in MCP (Model Context Protocol) server, allowing AI agents (Claude Desktop, Cursor, Windsurf, VS Code) to generate images, create videos, search ideas, and more directly in conversation.

## Quick Start

Add to your MCP config file:

```json
{
  "mcpServers": {
    "aigc-cli": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

For Claude Desktop: `~/Library/Application Support/Claude/claude_desktop_config.json`
For Cursor/Windsurf: `.cursor/mcp.json` or Settings → MCP

## Available Tools

| Tool | Description | Cost |
|---|---|---|
| `generate_image` | Generate images (text-to-image, image-to-image, inpainting); optional whitelisted `provider` | Paid (API key) |
| `generate_video` | Generate videos (async submit → poll); always uses `defaults.video.provider`, no `provider` argument | Paid (API key) |
| `generate_music` | Generate music (APIMart suno/flowmusic async, OpenRouter Lyria, Alibaba Cloud Bailian Fun-Music); optional `provider` | Paid (API key) |
| `generate_speech` | Text-to-speech; optional `provider` / `model` / `voice` / `format` | Paid (API key) |
| `transcribe_audio` | Speech-to-text for a local audio file; optional `provider` / `model` / `language` | Paid (API key) |
| `midjourney_imagine` | Midjourney artistic image generation (5-10x the cost of `generate_image`, 4 variants per job). Only for explicit Midjourney requests or highly artistic/stylized results — prefer `generate_image` otherwise | Paid (API key) |
| `midjourney_describe` | Image → reverse prompt (describe an image the way Midjourney would prompt it) | Paid (API key) |
| `midjourney_reroll` | Regenerate a Midjourney job (same prompt, new results), needs a previous MJ `task_id` | Paid (API key) |
| `midjourney_video` | Image → short video via Midjourney | Paid (API key) |
| `list_models` | List marketplace models, filterable by `type`; no API key required | Free (no key) |
| `get_model_pricing` | Query pricing details for a specific model; no API key required | Free (no key) |
| `get_balance` | Query API key and account balance | Free (API key) |
| `get_task` | Query async task / OpenRouter video job status and results | Free (API key) |
| `get_config` | Current effective configuration (secrets masked), for confirming generation parameters | Free (local) |
| `caption_image` | Read or write the caption/description of an image (JPEG/PNG) | Free (local) |
| `search_ideas` | Search the local prompt idea library (keywords, or `random=true`) | Free (local) |
| `remove_background` | Offline background removal (RMBG 2.0); optional `replace_color` / `replace_image` / `autocrop` | Free (local) |
| `convert_depth` | Image/video → grayscale depth map (Depth Anything V2, local ONNX); `annotate` overlays skeleton/face landmarks | Free (local) |
| `detect_image` | Detect C2PA / SynthID / TC260 / EXIF AIGC signals (fully offline) | Free (local) |
| `remove_watermark` | Detect and remove a visible AI watermark (built-in gemini; others via `learn-watermark`), output `<input>_clean<ext>` | Free (local) |
| `add_watermark` | Add a visible AI watermark (test fixtures only; built-in gemini alpha map, unknown names rendered as text) | Free (local) |
| `crop_watermark` | Crop to remove watermarks (no learning required; `target` = auto / n% / WxH) | Free (local) |
| `recognize_text` | Offline OCR for images/PDFs (requires `aigc-cli ocr init`; markdown or json output) | Free (local) |
| `kb_find` | Search the local knowledge base (keyword + semantic) | Free (local) |
| `kb_search` | Web search (`provider` selects the search engine: duckduckgo/firecrawl) and save results to the KB | Free (web) |
| `kb_add` | Add a local file to the knowledge base (.md/.txt/.go/.py/.json/.yaml/.html) | Free (local) |
| `kb_fetch` | Fetch a URL, convert it to markdown, and save it to the KB | Free (web) |
| `kb_list` | List all documents in the knowledge base | Free (local) |
| `kb_show` | Show the full content of a knowledge base document by ID (first 12 chars) | Free (local) |

> Every tool declares MCP annotations (`readOnlyHint` / `destructiveHint` / `idempotentHint` / `openWorldHint`): hosts can skip confirmation for read-only tools, and no tool in this server is destructive.
>
> Tools declared read-only (`readOnlyHint=true`, hosts may skip confirmation): `list_models`, `get_model_pricing`, `get_balance`, `get_task`, `get_config`, `caption_image`, `search_ideas`, `detect_image`, `recognize_text`, `kb_find`, `kb_list`, `kb_show`.

> `generate_image` / `generate_video` / `generate_music` share the CLI provider routing, so every supported provider (OpenRouter, APIMart, Agnes, OpenLux, ModelScope, Gemini, ZeekAI, Alibaba Cloud Bailian, …) works without extra configuration.
>
> `midjourney_*` tools resolve their provider from `defaults.midjourney.provider` (or the global `api_key`/`base_url`), and `midjourney_imagine` merges the `defaults.midjourney.*` settings.
>
> `generate_image` / `generate_music` / `generate_speech` / `transcribe_audio` and the `midjourney_*` tools also accept an optional `provider` argument restricted to names from `config.providers` (whitelist); `generate_video` does not — it always uses `defaults.video.provider`. See "Per-call Provider" below.
>
> Image parameters are pinned by config: `defaults.image.*` overrides the same parameter passed by the agent unless `defaults.chat.allow_tool_override: true` is set.
>
> Media-producing tools (`generate_image`, `generate_speech`, `convert_depth`, `remove_background`, `remove_watermark`, `add_watermark`, `crop_watermark`) additionally return **inline image/audio content blocks** next to the text result, so hosts such as Claude Desktop can render generated media directly in the conversation; video and other media MCP cannot represent stay text-only with the file path. At most 4 files (each ≤ 4 MiB) are inlined per result, with an extra text note when files are skipped. Set `AIGC_MCP_EMBED_MEDIA=0` (or `false`/`off`) to disable inlining and return text paths only.
>
> When the host passes a `progressToken` (e.g. Cursor), the async `generate_video` / `generate_music` tools emit **coarse-phase progress notifications** (0.05 before dispatch, 0.40 entering the submit → poll phase) so the host can render a progress indicator. Only these two milestones are emitted today — per-poll percentages are not yet available. Without a `progressToken` the tools stay fully silent.

### Per-call Provider (Whitelist)

`generate_image`, `generate_music`, `generate_speech`, `transcribe_audio`, and the four `midjourney_*` tools accept an optional `provider` argument:

- **Omitted (default)**: behaves exactly as before — the provider selected by `defaults.{command}.provider` (falling back to the global `api_key` / `base_url`).
- **Provided**: must be the **name** of a provider defined in `config.providers` (whitelist). URLs, paths, and names that are not configured are rejected with an error.

> `generate_video` does **not** accept `provider`: it always uses `defaults.video.provider` (falling back to the global `api_key` / `base_url`).

```yaml
# ~/.config/aigc-cli/config.yaml
providers:
  my-openrouter:
    type: openai
    api_key: sk-or-xxx
    base_url: https://openrouter.ai/api/v1

defaults:
  image:
    provider: my-openrouter
```

```json
{ "prompt": "a cat under the stars", "provider": "my-openrouter" }
```

**Security**: MCP tools never accept `base_url` or `api_key` arguments, and no tool can write config (there is no `set_config`). Agents can only reference provider names you configured and trust in `config.yaml`; they cannot point requests at an arbitrary host or inject credentials.

### What the `model` argument actually does

- `generate_image` / `generate_video`: **config wins** — `defaults.image.model` / `defaults.video.model` overrides the agent-supplied `model` unless `defaults.chat.allow_tool_override: true` is set (then the agent value wins and config only fills gaps).
- `generate_music` / `generate_speech` / `transcribe_audio`: the agent-supplied `model` is used as given; `defaults.music.model` / `defaults.audio.speak_model` / `defaults.audio.transcribe_model` fills in only when omitted (then code defaults).

## Tool Filtering

Control which tools are available via config:

```yaml
# Allowlist — only free/offline tools
tools_enable:
  - "search_ideas"
  - "detect_image"
  - "remove_watermark"
  - "crop_watermark"
  - "remove_background"
  - "convert_depth"
  - "recognize_text"

# Blocklist — disable paid tools
tools_disable:
  - "generate_image"
  - "generate_video"
  - "midjourney_*"
```

### Tool surface decision (design note)

`web_fetch`, `grep`, `read_file`, and `find` exist as **chat-only** agent tools (they live in `internal/cli/chat`, used by the interactive REPL / agent loop) and are **deliberately not exposed over MCP**: they hand an agent unsandboxed arbitrary file and web access, so a prompt-injection payload in a model-visible page or file could turn into local file disclosure. The MCP surface is intentionally scoped to AIGC capability (generation, detection, OCR, knowledge base, provider/config inspection). This is a deliberate boundary, not an oversight.

### Security hardening (v3.3.0)

After a pre-release security audit (attack-surface mapping + three hunter lanes + two independent PoC engineers), the MCP surface gained these defenses:

- **`output_path` confinement**: explicit output paths for the watermark / background / depth tools must resolve inside `output_dir` (`cfg.Output`) or the **input file's directory**; symlink targets are rejected (no write-through-symlink truncation). Omitting `output_path` keeps the default "next to the input" behavior unchanged.
- **Local-image validation**: local files referenced via `image_urls` must be **decodable images (≤32 MiB)**; non-image files (e.g. `~/.ssh/id_rsa`, `config.yaml`) are never inlined into requests sent to the provider.
- **Decode-bomb guard**: local image tools read the header size first and refuse >100 megapixels.
- **`generate_speech.format` enum**: only `mp3/wav/opus/aac/flac/pcm`, closing the `speech_<ts>.<ext>` path traversal.
- **Task-ID sanitization**: `get_task` download filenames use a `filepath.Base`-sanitized token, blocking `task_id` directory traversal.
- **KB path fix**: `kb_*` tools now use `~/.config/aigc-cli/knowledge` (same as CLI/chat), removing the literal-`~` directory and the pre-planted-KB poisoning vector.
- **KB network egress**: `kb_fetch` / `kb_search` use the global HTTP client (honoring `http_proxy`) and **reject loopback / private / link-local / metadata addresses** (re-validated on every redirect); response bodies are size-capped.
- Fixed the MCP server startup panic when the config has no `defaults:` section.

## Prompts / Workflow Templates

MCP Prompts are clickable workflow templates in hosts such as Claude Desktop and Cursor. Selecting one sends a pre-filled instruction that tells the agent which MCP tool to call and how to complete the whole flow.

| Prompt | Arguments | What it does |
|---|---|---|
| `generate_product_shot` | `product` (required), `style` (optional), `aspect_ratio` (optional) | Commercial product photo: calls `generate_image` with studio lighting, a clean seamless background, and an advertising-quality look, applying style / aspect ratio when given |
| `detect_ai_image` | `file_path` (required) | AI-image check: calls `detect_image`, then explains the fused C2PA / TC260 / SynthID / FFT / ONNX / visible-watermark signals and gives a clear verdict |
| `image_to_video` | `image_url` (required), `prompt` (optional) | Animate a still image: calls `generate_video` with the image as reference and an optional motion prompt, then reports the saved file |
| `remove_image_background` | `file_path` (required), `replace_color` (optional) | Offline background removal: calls `remove_background`, replacing the background color only when `replace_color` is given, then reports the output path |

When a required argument is missing, the prompt still returns the template and tells the agent to ask the user for the missing value.

List all registered prompts without starting the server:

```bash
aigc-cli mcp --list-prompts
```

## Resources

MCP Resources are read-only data sources hosts can attach directly (unlike tools, they never cause side effects).

| Resource URI | Kind | Contents |
|---|---|---|
| `aigc://providers` | static | Names and hosts of the providers configured in `config.providers`, so agents can pick a valid value for the `provider` tool argument. API keys are never included. |
| `aigc://config` | static | The effective configuration with all secrets masked (same guarantee as `--print-config` / the `get_config` tool). |
| `aigc://output` | static | Top-level listing of the output directory (name, size, modified time). Non-recursive, capped at 200 entries with a truncation note. |
| `aigc://output/{filename}` | template | Reads a single file from the output directory: images/audio as base64 blobs, `.txt/.md/.json/.yaml/.yml/.csv/.log` as text. |

Security constraints (read-only):

- Only plain files directly inside the configured output directory are readable; `../`, `/`, `\`, absolute paths, volume names, and symlinks are rejected.
- Files larger than 4 MiB are refused; unsupported file types return a clear error.
- With no output directory configured, `aigc://output` reports that and the template errors.
- No resource ever exposes API keys or other credentials.

## Configuration

MCP mode uses the same config.yaml as CLI mode. Provider, API keys, and defaults are shared.

## Testing

List available tools without starting the server:

```bash
aigc-cli mcp --list-tools
```

List available prompts the same way:

```bash
aigc-cli mcp --list-prompts
```
