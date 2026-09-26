# Music Generation

`aigc-cli music` generates music from a natural-language prompt. The backend is selected automatically from the provider:

- **APIMart (default)** — async task model: submit → poll → download. Use `--model suno` (default) or `--model flowmusic`.
- **OpenRouter** — synchronous streaming: `--model google/lyria-3-clip-preview` (default, ~30s, mp3) or `google/lyria-3-pro-preview` (~minutes, mp3/wav). The audio is returned in a single call with **no task ID**.
- **Alibaba Cloud Bailian (Fun-Music)** — synchronous: a single call returns an audio URL (valid 24 hours), with **no task ID**. Use `--model fun-music-v1` (default) or `fun-music-preview`.

## Commands

```
aigc-cli music
├── generate / gen   Submit a music generation task
└── query <task-id>  Query task status and download the result
```

## Basic Usage

```bash
# APIMart suno (default): generate from a description
aigc-cli music generate --prompt "city pop"

# Alias gen
aigc-cli music gen --prompt "city pop"

# flowmusic backend
aigc-cli music gen --prompt "rock" --model flowmusic

# Custom lyrics (suno switches to custom mode) + title
aigc-cli music gen --prompt "pop" --lyrics "la la la" --title "My Song"

# Instrumental only (no vocals)
aigc-cli music gen --prompt "lofi beats" --instrumental

# Duration and format (suno)
aigc-cli music gen --prompt "epic orchestral" --duration 120 --format mp3

# Alibaba Cloud Bailian Fun-Music (synchronous)
aigc-cli music gen --provider dashscope --model fun-music-v1 --prompt "summer folk"
aigc-cli music gen --provider dashscope --json '{"model":"fun-music-v1","input":{"prompt":"rock","gender":"male"}}'

# Query a task: downloads the audio when complete (APIMart only)
aigc-cli music query task_xxx
```

---

## Provider Differences

| Provider | Model | Mode | Request fields |
|---|---|---|---|
| APIMart (default) | `suno` (default) | Async: submit → poll → download | prompt / style / title / lyrics / instrumental / duration / format |
| APIMart | `flowmusic` | Async: submit → poll → download | sound_prompt / lyrics / title / length |
| OpenRouter | `google/lyria-3-clip-preview` (default) | Sync streaming, audio returned in one call | ~30s, mp3 |
| OpenRouter | `google/lyria-3-pro-preview` | Sync streaming | minutes, mp3 / wav |
| Bailian | `fun-music-v1` (default) / `fun-music-preview` | Sync, one call returns an audio URL | prompt / lyrics / instrumental / format / gender |

> If `--model` contains `flowmusic`, the flowmusic backend is used; otherwise suno is used. This rule applies to APIMart only.
> The provider is detected from the base URL: `*.maas.aliyuncs.com` or `dashscope.aliyuncs.com` → Alibaba Cloud Bailian.

### APIMart (async)

The submission returns a `task_id`; the CLI polls until completion and downloads the result. `music query <task-id>` inspects an in-flight task or re-fetches a past one.

Field mapping:

- **suno**
  - `--prompt` (or `--style`) → `prompt` (no lyrics) or `style` (with lyrics)
  - non-empty `--lyrics` → `custom: true`, lyrics written to `prompt`
  - `--title` → `title`
  - `--instrumental` → `instrumental`
  - `--duration` → `duration`
  - `--format` → `audio_format`
- **flowmusic**
  - `--prompt` (or `--style`) → `sound_prompt`
  - `--lyrics` → `lyrics` (omitted when `--instrumental`)
  - `--title` → `title`
  - `--duration` → `length` (defaults to 120 seconds)

> Both backends require usable input: suno needs a `prompt` (i.e. `--prompt` / `--style` / `--lyrics`); flowmusic needs `sound_prompt` or `lyrics`.

### OpenRouter (synchronous streaming)

No named provider is required — point at OpenRouter via environment variables or `--api-base`:

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

# Default model (~30s, mp3)
aigc-cli music gen --prompt "ambient"

# Pro model (minutes; mp3 / wav)
aigc-cli music gen --model google/lyria-3-pro-preview --prompt "cinematic" --format wav
```

Or reference a named provider from your config:

```bash
aigc-cli music gen --provider openrouter --prompt "ambient"
```

Notes:

- Requires an OpenRouter API key; the base URL is `https://openrouter.ai/api/v1`.
- Synchronous — there is **no task ID**, so `music query` is neither needed nor supported.
- OpenRouter has no lyrics / instrumental / duration request fields, so these are folded into the prompt text: instrumental is prefixed with `[Instrumental] `, lyrics are appended in a `Lyrics:` section, and duration is appended as `Target duration: about N seconds.`.
- Add the global `--zdr` flag to send `provider: {zdr: true, data_collection: "deny"}` and restrict routing to ZDR-compliant upstreams; the request fails if none is available.

### Alibaba Cloud Bailian Fun-Music (synchronous)

Vendor docs: <https://docs.bailian.console.aliyun.com/zh/model-studio/fun-music>

```bash
aigc-cli music gen --provider dashscope --model fun-music-v1 --prompt "summer folk"
```

Notes:

- **Endpoint**: `POST https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/api/v1/services/audio/music/generation`.
  This is the DashScope **native** path, **not** the `compatible-mode/v1` path used for chat. The CLI takes only the scheme + host from the base URL and appends the native path, so the same named provider used for chat works unchanged.
- Auth: `Authorization: Bearer <DASHSCOPE_API_KEY>`.
- **Synchronous**: one call returns the audio URL (valid for 24 hours). There is **no task ID**, so `music query` is neither needed nor supported.
- Model differences:
  - `fun-music-v1`: at least one of `prompt` / `lyrics`; supports `gender` (male/female).
  - `fun-music-preview`: `prompt` is required; `gender` is not supported.
- Field mapping: `--prompt` (or `--style`, joined with `，` when both are given) → `input.prompt`; `--lyrics` → `input.lyrics`; `--instrumental` → `input.is_instrumental`; `--format` → `input.format`.
- **No equivalent fields**: `--duration` and `--title` are ignored (the length is derived from the lyrics) and the CLI prints a warning.
- Vendor-specific fields (`gender`, `enable_aigc_watermark`, …) go through `--json`; with no flags accompanying it, `--json` is forwarded **verbatim**, so write the full DashScope-native shape (including the `input` nesting).
- When `is_instrumental=true`, `lyrics` and `gender` are invalid and the CLI removes them automatically.
- Region/availability: the model is currently in **limited preview**, available **only in China (Beijing)**, and requires approval in the Bailian Model Gallery.

---

## Parameters

| Parameter | Short | Description |
|---|---|---|
| `--prompt` | `-p` | Music description / style (suno: prompt when no lyrics; flowmusic: sound_prompt) |
| `--model` | `-m` | Backend model: `suno` (default) / `flowmusic` / `fun-music-v1` / `fun-music-preview`; on OpenRouter, the model ID |
| `--style` | | Style (suno's style field; fallback when prompt is empty; joined into the prompt on Bailian) |
| `--title` | | Track title (not supported by Bailian; ignored) |
| `--lyrics` | | Lyrics (suno non-empty enables custom mode; flowmusic / Bailian send as lyrics) |
| `--instrumental` | | Instrumental only (no vocals) |
| `--duration` | `-d` | Duration in seconds; suno → `duration`, flowmusic → `length`; not supported by Bailian |
| `--format` | | Audio format (suno / OpenRouter / Bailian; not supported by flowmusic) |
| `--json` | | JSON input (file path, string, or `-` for stdin); a backend-native body sent **verbatim** when no flags are set. Explicitly-set flags overlay the key the active backend uses |
| `--dry-run` | | Print the equivalent curl, do not call the API |
| `--provider` | `-P` | Global: reference a named provider (e.g. `openrouter`) |
| `--api-key` | | Global: override API key |
| `--api-base` | | Global: override base URL |
| `--output` | | Global: download directory (default: current directory) |

---

## `--json`: Backend-Native Body, Flag Overlay

`--json` is a **backend-native body**. The four backends use different keys, so write **that backend's native shape** — the CLI does not translate one backend's shape into another's:

| Backend | `--json` keys the CLI can target |
|---|---|
| APIMart `suno` | `prompt` / `style` / `title` / `duration` / `audio_format` |
| APIMart `flowmusic` | `sound_prompt` / `length` |
| OpenRouter Lyria | folded into `messages[0].content` + `audio.format` |
| Bailian fun-music | `input.prompt` / `input.lyrics` / `input.format` / `input.is_instrumental` |

With `--json` and no flags, the body is sent byte-for-byte unchanged:

```bash
# suno-specific fields (native flat shape)
aigc-cli music gen --provider apimart --json '{"model":"suno","prompt":"city pop","style_weight":0.6,"vocal_gender":"Female"}'

# flowmusic-specific fields
aigc-cli music gen --provider apimart --json '{"model":"flowmusic","sound_prompt":"rock","length":120,"bpm":"128","seed":"42"}'

# Bailian Fun-Music: native input nesting
aigc-cli music gen --provider dashscope --json '{"model":"fun-music-v1","input":{"prompt":"urban folk","gender":"male"}}'

# OpenRouter (chat/completions shape)
aigc-cli music gen --provider openrouter --json '{"model":"google/lyria-3-pro-preview","messages":[{"role":"user","content":"ambient"}],"modalities":["text","audio"],"stream":true}'
```

> 💡 **JSONC is accepted.** `//` line comments, `/* */` block comments, trailing commas, and a leading UTF-8 BOM are stripped before parsing. The stripping is string-literal aware, so a `//` inside a value such as `"https://host/path"` is preserved, and a comment-free strict JSON body is unchanged byte-for-byte.

When you also pass flags, only the flags you explicitly set overlay the key the **active backend** would use; every key you did not touch is preserved exactly.

```bash
# JSON holds the model + lyrics; style and title come from flags
aigc-cli music gen --provider apimart --json downloads/track.json --style "city pop" --title "Neon Rain"
```

> ⚠️ A backend only has a target for the keys it actually uses. Some flags are silently dropped when the active backend has no matching key: `--format` has no target for flowmusic, and `--duration` / `--title` have none for Bailian fun-music. On suno, `--prompt` maps to `style` when lyrics are present, otherwise to `prompt`.

> 💡 `--dry-run` / `--verbose` print the real endpoint and real body, so you can verify the overlay directly.
> 💡 Endpoints differ per backend: APIMart `{base}/music/generations`, OpenRouter `{base}/chat/completions`, Bailian's native DashScope `/api/v1/services/audio/music/generation`.

---

## Configuration

`~/.config/aigc-cli/config.yaml`:

```yaml
providers:
  apimart:  { type: openai, api_key: "sk-xxx", base_url: "https://api.apimart.ai" }
  openrouter: { type: openai, api_key: "sk-or-xxx", base_url: "https://openrouter.ai/api/v1" }
  # Bailian: chat uses compatible-mode; music is redirected to the native services path
  dashscope: { api_key: "sk-xxx", base_url: "https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/compatible-mode/v1" }
defaults:
  music:
    provider: apimart        # set to dashscope to enable Fun-Music
    model: suno              # with dashscope use fun-music-v1 or fun-music-preview
    # instrumental: false
    # duration: 120
    # format: mp3
```

---

## `--dry-run`

Prints the equivalent curl without calling the API.

APIMart (suno default):

```bash
aigc-cli music gen --prompt "city pop" --dry-run
```

```
curl -X POST https://api.apimart.ai/v1/music/generations \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{"custom":false,"instrumental":false,"model":"suno","prompt":"city pop","version":"v6"}'
```

APIMart (flowmusic):

```
curl -X POST https://api.apimart.ai/v1/music/generations \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{"length":120,"model":"flowmusic","sound_prompt":"rock"}'
```

OpenRouter (Lyria, synchronous streaming):

```
curl -N -X POST https://openrouter.ai/api/v1/chat/completions \
  -H "Authorization: Bearer sk-or-xxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"google/lyria-3-clip-preview","messages":[{"role":"user","content":"ambient"}],"modalities":["text","audio"],"audio":{"format":"mp3"},"stream":true}'
```

Alibaba Cloud Bailian (Fun-Music, synchronous; note the native services path):

```
curl -X POST 'https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/api/v1/services/audio/music/generation' \
  -H "Authorization: Bearer sk-xxx" \
  -H "Content-Type: application/json" \
  -d '{"input":{"prompt":"urban folk"},"model":"fun-music-v1"}'
```

---

## MCP / Chat Agent

Music generation is exposed to the MCP Server and the Chat Agent, so an AI agent can call the `generate_music` tool to produce music in conversation. See [guide-mcp.md](guide-mcp.md) and [guide-chat.md](guide-chat.md).

---

## Output

Generated audio is saved to the output directory (current directory by default; override with `--output`) and the CLI prints `Saved: <path>`:

- **APIMart**: each track is saved as `music_<task_id>_<n>.<ext>`.
- **OpenRouter**: saved as `audio_<unix timestamp>.mp3` (or `.wav` with `--format wav`).
- **Alibaba Cloud Bailian**: saved as `music_<audio_id>_0.<ext>`.

---

## API Reference

| Endpoint | Purpose | Applies to |
|---|---|---|
| `POST /v1/music/generations` | Submit a music generation task (async) | APIMart ✅ |
| `GET /v1/music/tasks/{task_id}` | Query music task status and result | APIMart ✅ |
| `POST /v1/chat/completions` | Synchronous streaming music generation (`modalities: ["text","audio"]`) | OpenRouter ✅ |
| `POST /api/v1/services/audio/music/generation` | Synchronous music generation (DashScope native protocol, returns an audio URL) | Alibaba Cloud Bailian ✅ |
