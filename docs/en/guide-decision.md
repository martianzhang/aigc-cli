# Decision Models

`aigc-cli decision` (aliases `decide`, `systemone`) runs typed "decision" questions (single-choice / yes-no / rating) against a block of text and returns structured answers with probabilities. This command does **not** generate free-form text. The response is JSON — pipe it to `jq` for a readable view (see [Output](#output)).

## Command

```
aigc-cli decision [flags]
```

## Flags

| Flag | Description |
|---|---|
| `--json <path\|inline-json\|->` | Question bank (JSON or JSONC). Accepts a file path, an inline JSON string, or `-` for stdin. Falls back to `defaults.decision.bank` when omitted |
| `--questions a,b,c` | Filter which named questions to answer (comma-separated, repeatable). Omit to answer **all** questions in the bank |
| `--state <text\|path\|->` | Material to judge. Accepts literal text, a file path, or `-` for stdin. If the content is a valid JSON object or array, it is sent as structured state; otherwise as a plain string |
| `--image <path\|data-uri>` | Attach an image (PNG/JPEG/WebP) scored jointly with the state; repeatable. Accepts a local file or a base64 data URI. Requires Clef / Clef-Flash on Ollama >= 0.35.1 |
| `--audio <path\|data-uri>` | Attach an audio clip (WAV/MP3/M4A/AAC/OGG/OPUS/FLAC) scored jointly with the state; repeatable. Accepts a local file or a base64 data URI. Requires Clef-Omni / a server that supports audio |
| `--video <path\|data-uri>` | Attach a video clip (MP4/MOV/WebM/MKV/AVI), sampled at 2 fps with its soundtrack; repeatable. Accepts a local file or a base64 data URI. Requires Clef-Omni / a server that supports video |
| `--image-resize <px>` | Downscale attached images so the longest edge is at most N px before sending. Default `1024`; `0` keeps the original. This is what actually cuts latency — see [Images](#images-multimodal) |
| `--list` | List the question names available in the bank, then exit |
| `--dry-run` | Print the equivalent curl request without calling the API |
| `-P, --provider` | (global) Switch named provider, e.g. `ollama` or `openrouter` |
| `-m, --model` | (global) Switch model, e.g. `tev1`, `tev1:0.8b`, `nimble`, `typesafe/jev-latest` |

## Question bank

The bank file is JSON or JSONC (`//` comments and trailing commas allowed). Top-level fields:

| Field | Type | Required | Description |
|---|---|---|---|
| `model` | string | no | Overrides the default model |
| `state` | string / object / array | no | Default state; overridden by `--state` |
| `images` | array of strings | no | Default images (local paths or base64 data URIs); overridden by `--image` |
| `audio` | array of strings | no | Default audio clips (local paths or base64 data URIs); overridden by `--audio` |
| `videos` | array of strings | no | Default video clips (local paths or base64 data URIs); overridden by `--video` |
| `questions` | object | yes | Named question map, 1–64 entries |

Each question has:

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | `choice` / `noul` / `score` | yes | Question type |
| `instructions` | string | yes | The question text |
| `criteria` | object / array | depends | `choice` → `{ option: description }`; `score` → `[low → high, …]` (lowest first); `noul` → optional |

A full example is in [examples/decision/triage.json](examples/decision/triage.json), paired with [examples/decision/state.txt](examples/decision/state.txt).

### Three question types and answers

| Type | Returned fields |
|---|---|
| `choice` | `choice` (chosen option key), `probabilities` (per option), `confidence` |
| `noul` | `noul` (probability the answer is true, 0–1). **No** `confidence` field |
| `score` | `score` (probability-weighted level, may be fractional), `legend` (index → label), `probabilities`, `confidence` |

> `confidence` measures how concentrated the probability distribution is, **not** the chance the answer is correct.

### Images (multimodal)

Decision models (Clef / Clef-Flash, Ollama >= 0.35.1) can also judge images. Attach them with `--image` (repeatable); every image is shared by **all** questions in the request and scored jointly with the text `state`:

```bash
aigc-cli decision -m clef-flash --image form.png --questions complete,type --state "The agent wants to submit the attached form."
```

Images are sent in request order. Local PNG/JPEG/WebP files are encoded automatically; base64 `data:` URIs are stripped to raw base64.

#### Latency and `--image-resize`

A decision model's cost is driven by **pixel dimensions, not file size** — the image becomes vision tokens, and the model scores everything in one forward pass. Re-compressing an image (lower quality or smaller file) does **not** help; only downscaling does:

| Input | input_tokens | Latency |
|---|---|---|
| text only | 164 | ~1s |
| 2848×1600 PNG (2.4 MB) | ~4100 | ~47s |
| same image re-encoded q75/q40 (same pixels) | ~4100 | ~47s |
| **2848×1600 downscaled to 1024px** | ~800 | ~6s |
| downscaled to 768px | ~570 | ~4s |

`--image-resize` (default `1024`) downscales the longest edge before sending, preserving aspect ratio and format, and never upscaling. Pass `--image-resize 0` to send originals, or a larger value (e.g. `1600`) when fine detail matters.

```bash
# A large screenshot is auto-downscaled to 1024px → much faster
aigc-cli decision -m clef-flash --image screenshot.png --state "Judge the attached screenshot."
aigc-cli decision -m clef-flash --image screenshot.png --image-resize 768 --state "Judge the attached screenshot."
```

### Audio and video (multimodal)

Clef-Omni / the Clef family can also judge audio and video. Attach clips with `--audio` and `--video` (both repeatable); every clip is shared by **all** questions and scored jointly with the text `state` and any images:

```bash
aigc-cli decision -m clef-omni --audio call.wav --questions escalation --state "Review the attached call recording."
aigc-cli decision -m clef-omni --video dashcam.mp4 --questions collision --state "Does the clip show a collision?"
```

Unlike images (sent as raw base64), audio and video are sent as **base64 data URLs** (`data:audio/wav;base64,...`, `data:video/mp4;base64,...`) so the MIME type travels with the bytes. Local files are typed from their extension: `.wav .mp3 .m4a .aac .ogg .oga .opus .flac .webm` for audio and `.mp4 .m4v .mov .webm .mkv .avi` for video (up to 64 MiB each). A `data:` URI passes through unchanged; `http(s)` URLs are rejected — download the file first. Video is sampled at **2 frames per second** server-side, and a clip's soundtrack is heard when every video in the request has one.

> Audio/video needs a model server that supports it. On OpenRouter, `cloudflare/clef-omni` currently accepts **text and images only** — audio/video input is "coming soon". The HuggingFace release (`Cloudflare/clef-omni`) itself supports all three when self-hosted.

## Output

The response is JSON (`model` / `answers` / `usage`) and pipes straight into `jq`:

```bash
# The chosen option for one question
aigc-cli decision --json triage.json --state state.txt | jq -r '.answers.intent.choice'

# One line per answer (key-agnostic; the three fields are mutually exclusive, so // falls back in turn)
aigc-cli decision --json triage.json --state state.txt \
  | jq -r '.answers | to_entries[] | "\(.key): \(.value.choice // .value.noul // .value.score)"'

# Act only when the model is confident, otherwise escalate
aigc-cli decision --json triage.json --state state.txt \
  | jq -r 'if .answers.intent.confidence > 0.85 then .answers.intent.choice else "ESCALATE" end'
```

> 💡 Question names and `criteria` keys are free-form strings, so a bank may use non-ASCII names. Dot notation only works for ASCII identifiers: with a Chinese key, use brackets (`.["answers"]["意图"].choice`) — jq rejects `.answers.意图.choice` with a syntax error. The Chinese bank in `docs/zh/examples/decision/triage.json` shows a fully translated example.

## Supported providers

Endpoint paths **differ per provider**. This project has only end-to-end-verified **Ollama** and **OpenRouter**; the others are documented per their official docs but **untested here**.

| Provider | Official docs | Configured `base_url` | Full endpoint |
|---|---|---|---|
| TypeSafe AI | https://docs.typesafe.ai/api | https://api.typesafe.ai | POST https://api.typesafe.ai/v1/systemone |
| OpenRouter | https://openrouter.ai/docs/guides/community/typesafe-sdk | https://openrouter.ai/api/v1 | POST https://openrouter.ai/api/v1/systemone |
| Ollama (>= 0.35.0; images need >= 0.35.1) | https://ollama.com/library/tev1 | http://localhost:11434 | POST http://localhost:11434/v1/systemone |
| LLM Gateway | https://docs.llmgateway.io/features/system-one | https://api.llmgateway.io/v1 | POST https://api.llmgateway.io/v1/systemone |
| LiteLLM proxy | https://docs.litellm.ai/docs/pass_through/typesafe | `{proxy}/typesafe` | POST `{proxy}/typesafe/v1/systemone` |

- **Ollama** needs no API key and runs fully locally.
- **TypeSafe AI** and **OpenRouter** (and the others) require an API key.

## Priority

```
CLI flags > JSON (bank) > defaults.decision YAML > code defaults
```

When `--json` ships alongside other flags, every flag you **explicitly set** overrides the matching key in the bank; untouched fields are kept as written. With `--json` alone, the bank is sent verbatim. `--image` / `--audio` / `--video` override the bank's `images` / `audio` / `videos` arrays.

## Examples

> These examples assume `defaults.decision.provider` is set (e.g. `ollama`). If it is not, pass `-P ollama` (local) or `-P openrouter`.

```bash
# Local Ollama (free, offline)
aigc-cli decision --json triage.json --questions intent,refund --state state.txt

# Switch to a remote Jev model on OpenRouter
aigc-cli decision -P openrouter -m typesafe/jev-latest --questions intent --state state.txt

# Smaller local model (uses the default bank from config)
aigc-cli decision -m tev1:0.8b --state state.txt

# Structured state (object) — label the context sections
aigc-cli decision --questions refund \
  --state '{"policy":"Refunds within 30 days.","request":"Bought 12 days ago, want a refund."}'

# stdin, list, dry-run
cat ticket.txt | aigc-cli decision --questions intent
aigc-cli decision --list
aigc-cli decision --questions intent --state state.txt --dry-run
```

## Caveats

- `choice` and `score` accept 2–26 options; tev1 was trained on 2–24, so **stay in 2–24**.
- 1–64 questions per request; request body ≤ 64 KiB.
- tev1 runs at ~2000 tokens of context — keep the state short.
- Images require **Clef / Clef-Flash** (Ollama >= 0.35.1); text-only models reject them. The API accepts only **raw base64**: local files are encoded for you, `data:` URIs are stripped/converted, and `http(s)` URLs are **rejected** — download the image first. A bad image file (corrupt, non-image, > 32 MiB) is rejected before the request is sent.
- Image latency is driven by **pixel dimensions, not file bytes** — only `--image-resize` (default 1024px) speeds things up. Re-compressing (lower quality / smaller file) does **not** reduce tokens. Set `--image-resize 0` to send originals.
- Audio/video need **Clef-Omni / Clef** and a server that supports them. On OpenRouter `cloudflare/clef-omni` is **text + image only** today (audio/video "coming soon"), so `--audio` / `--video` work only against a self-hosted model or once the provider enables them. Clips are sent as base64 **data URLs** (extension-typed, ≤ 64 MiB each); `http(s)` URLs are rejected.
- If none of the options might fit, **add a `none` / `other` option**. The model cannot say "none of the above" unless you give it one.
- tev1 is built on a **Qwen3.5** base (`ollama show tev1` → `arch qwen35`), and non-English works in our smoke tests (see the Chinese bank in `docs/zh/examples/decision/triage.json`). The vendor's evaluation is English-only, though, so treat non-English as **unsupported** — pair it with a `confidence` threshold and a human fallback.
- Prompt-injection coverage is limited; treat the state as data, never as instructions.
- Decision models can be wrong. Do **not** rely on them as the sole check for high-stakes decisions.
- MCP exposure is **not** part of this command yet.

## Config

```yaml
defaults:
  decision:
    provider: ollama            # named provider from config.providers (ollama / openrouter / ...)
    model: tev1                 # tev1 | tev1:0.8b | nimble | typesafe/jev-latest | cloudflare/clef-omni
    bank: ~/exams/triage.json   # default question bank (used when --json is omitted)
```