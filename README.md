![aigc-cli](logo.svg)

[![CI](https://github.com/martianzhang/aigc-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/martianzhang/aigc-cli/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/martianzhang/aigc-cli)](https://go.dev/)
[![License](https://img.shields.io/github/license/martianzhang/aigc-cli)](LICENSE)
[![Release](https://img.shields.io/github/v/release/martianzhang/aigc-cli)](https://github.com/martianzhang/aigc-cli/releases)

**Terminal-native AIGC toolkit: multi-provider image/video/audio generation, Midjourney, chat, AIGC forensics, knowledge base, MCP Server.**

> [中文版文档](README_zh.md)

Generate, detect, and manage AI content from the terminal. Supports OpenAI, OpenRouter, Anthropic, Ollama and any compatible relay — with local offline models for AIGC detection, OCR, background removal, and knowledge base.

---

## Why aigc-cli?

| | | |
|---|---|---|
| 🤖 | **MCP Server** | Built-in MCP Server for Claude Desktop, Cursor, Windsurf. AI agents can generate images/videos, run Midjourney, search KB, detect AIGC, query pricing — all in conversation. |
| 🔬 | **AIGC Forensics** | Offline multi-signal fusion: C2PA, TC260 (GB 45438-2025), SynthID, ONNX classifier, FFT spectrum, SRM noise, JPEG quantization. Zero API key needed. |
| 🔌 | **Multi-Protocol** | Not just OpenAI — supports Anthropic Messages API, Ollama, local ONNX models alongside OpenAI-compatible endpoints. |
| 🧠 | **Provider Auto-Adapt** | Each provider gets the correct API routing automatically (OpenRouter dedicated image/video API, APIMart async tasks, etc.) |
| 🎨 | **Midjourney Pipeline** | 17 subcommands: imagine → blend → describe → upscale → zoom → inpaint → video → remix. No Discord required. |
| 💬 | **Agentic Chat** | Interactive REPL with built-in tools: `generate_image`, `generate_video`, `midjourney_*`, `ideas`, `kb_*`, `detect_image`, `recognize_text`. |
| 🔍 | **Prompt Ideas** | Offline BM25 search (CJK-aware + n-gram + RRF), 10K+ prompt dataset. |
| 🔊 | **Local TTS / ASR** | sherpa-onnx offline speech (kokoro 53 voices, EN/ZH/JA/KR/FR) and recognition (SenseVoice). No internet. |
| 🔄 | **Video Job Persistence** | OpenRouter submit → poll → download pipeline. `--job-id` resume after timeout. |
| 🧪 | **Dry-Run & Curl** | `--dry-run` prints the equivalent curl command for any API call. Learn and debug without cost. |
| ⚡ | **Go Binary** | `go install` one command. No runtime deps. Cross-platform. |
| 📚 | **Knowledge Base** | FTS5 + ONNX semantic search, age-encrypted vault, web search auto-import, MCP/Chat tools. |
| 👁️ | **OCR & Vision** | Offline DBNet+CRNN text recognition. Image captioning via local EXIF or online vision LLM. |
| 🖼️ | **Background Removal** | RMBG 2.0 semantic segmentation. Pure local ONNX, no API key. |


---

## Quick Start

### Installation

Download the binary for your platform from the [Releases page](https://github.com/martianzhang/aigc-cli/releases).

### Using OpenAI

```bash
export OPENAI_API_KEY="sk-xxx"

aigc-cli image --prompt "a cat under the stars"
aigc-cli chat --message "Hello, who are you?"
```

### Using OpenRouter (change env vars, commands stay the same)

```bash
export OPENAI_API_KEY="sk-or-xxx"
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"

aigc-cli image --model "openai/gpt-image-2" --prompt "a cat"
aigc-cli video --model "google/veo-3.1" --prompt "a dog running"     # auto-routes to dedicated video API
aigc-cli models --type image                                           # authentication-free model discovery
```

### Using any OpenAI-Compatible Relay

```bash
export OPENAI_API_KEY="sk-xxx"
export OPENAI_BASE_URL="https://your-relay.com/v1"

aigc-cli chat --message "Hello"
```

### MCP Integration (Recommended)

Add to your MCP config in Claude Desktop / Cursor / Windsurf:

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

AI agents can generate images, create videos, search idea libraries, query model pricing, and detect AIGC directly in conversation. See [docs/en/guide-mcp.md](docs/en/guide-mcp.md).

---

## Feature Overview

| | Capability | Description |
|---|---|---|
| 🤖 | **MCP Server** | Built-in MCP protocol support, works out of the box with Claude Desktop / Cursor / Windsurf / VS Code |
| 🔬 | **AIGC Detection Engine** | C2PA / TC260 / SynthID / ONNX / FFT / SRM noise / JPEG quantization, offline, emoji output |
| 🔌 | **Multi-Provider Unified Entry** | Change one `base_url` to switch providers, commands unchanged |
| 🧠 | **Provider Auto-Adapt** | OpenRouter automatically routes to dedicated image/video APIs, zero config |
| 🎨 | **Complete Midjourney Pipeline** | 17 subcommands covering imagine → blend → describe → upscale → zoom → inpaint → video → remix, no Discord needed |
| 💬 | **Agentic Chat** | Interactive REPL with built-in `generate_image` / `generate_video` / `midjourney_*` / `ideas` / `kb_*` tools |
| 🔍 | **Prompt Idea Library** | Offline BM25 search engine (CJK-aware + n-gram + RRF), 10K+ prompt dataset |
| 🔊 | **Local TTS / ASR** | sherpa-onnx offline speech synthesis (kokoro, 53 voices, EN/ZH/JA/KR/FR) and speech recognition (SenseVoice, best for Chinese), no internet needed |
| 🔄 | **Video Job Persistence** | OpenRouter submit → poll → download full pipeline, `--job-id` one-key resume after timeout |
| 🧪 | **Dry-Run & Curl** | `--dry-run` prints equivalent curl commands, zero-friction API learning and debugging |
| ⚡ | **Go Single Binary** | `go install` one-command install, no runtime dependencies, cross-platform |
| 📚 | **Local Knowledge Base** | FTS5 + ONNX semantic search, age-encrypted vault, web search auto-import, MCP/Chat tool integration |

---

## Provider Auto-Adaptation

The same `image` / `video` / `audio` / `models` command automatically uses the correct API path based on the provider:

| Provider | Image | Video | Audio | Models |
|---|---|---|---|---|
| **OpenAI** | `POST /v1/images/generations` (sync) | — | `POST /v1/audio/speech` + `POST /v1/audio/transcriptions` | `GET /v1/models` |
| **OpenRouter** | `POST /api/v1/images` (dedicated image API) | `POST /api/v1/videos` async → poll → download + `--job-id` resume | `POST /api/v1/audio/speech` + `POST /api/v1/audio/transcriptions` (10+ TTS model aggregation) | `GET /api/v1/images/models` / `GET /api/v1/videos/models` (auth-free) |
| **APIMart** | Async task submit → poll → download | Async task + VEO3 Remix (extend video) | `POST /v1/audio/speech` + `POST /v1/audio/transcriptions` | Marketplace API + model pricing query |
| **Yunwu AI** | — | `POST /v1/video/create` + `GET /v1/video/query` | ❌ Not yet available | — |
| **Ollama / Local** | `POST /v1/images/generations` (experimental, no API Key) | ❌ | Via LocalAI/openedai-speech etc. | `GET /v1/models` |
| **Anthropic** | — | — | `POST /v1/messages` (via Anthropic-compatible relay) | — |
| **Generic Relay** | `POST /v1/images/generations` (sync) | — | `POST /v1/audio/speech` (passthrough) | `GET /v1/models` |

> Local models/services don't need an API Key. aigc-cli auto-exempts API Key checks and skips the Authorization header. See [docs/en/installation.md#local-generation](docs/en/installation.md#local-generation).

Detection logic: auto-identifies by `base_url`, or manually specify with `--mode sync` / `--mode async`.
Each command can use a different provider via the `providers` config, see [docs/en/config.example.yaml](docs/en/config.example.yaml).

---

## Commands

```
aigc-cli
├── image / img   Image generation (sync/async/OpenRouter dedicated API/Grok Edit)  →  docs/en/guide-image.md
├── video / vid   Video generation (OpenRouter / Yunwu + VEO3 Remix)                →  docs/en/guide-video.md
├── audio / voice Audio: TTS and STT                                                →  docs/en/guide-audio.md
│   ├── tts / speak  Text-to-speech (cloud API or local sherpa-onnx offline)
│   ├── asr / stt    Speech-to-text (cloud API or local sherpa-onnx offline)
│   └── init         Download local models (kokoro, sense-voice, etc.)
├── ocr            Offline text recognition (DBNet + CRNN, ONNX local inference)     →  docs/en/guide-ocr.md
│   ├── init        Download OCR models
│   └── scan        Recognize text in images
├── background / bg AI background removal (RMBG 2.0 semantic segmentation, offline ONNX)  →  docs/en/guide-background.md
├── midjourney / mj                                                                  →  docs/en/guide-midjourney.md
│   └── mj     Alias for midjourney
├── chat      AI chat / Interactive REPL / Agent Loop (tool calling)                  →  docs/en/guide-chat.md
├── ideas / idea  Prompt idea search (keyword / random, defaults to random)           →  docs/en/guide-ideas.md
├── knowledgebase / kb  Local knowledge base (FTS5 + semantic search + ONNX embedding) →  docs/en/guide-knowledgebase.md
├── models / model                                                                    →  docs/en/guide-commands.md
│   └── --price    View model pricing
├── task       Query async task status (APIMart compatible)
├── balance    Query balance (APIMart compatible)
├── preview / pr View images / --detail metadata / --describe caption                 →  docs/en/guide-preview.md
├── detect     Detect AIGC, metadata and tampering (multi-signal fusion + emoji)     →  docs/en/guide-detect.md
├── completion Generate shell completion scripts (bash/zsh/fish/powershell)
├── mcp        Start MCP Server (AI agent integration)                                →  docs/en/guide-mcp.md
│
│   # Global flags
│   --dry-run      Print request params and equivalent curl, no API call
│   --print-config Print effective config with source annotations
│   -v/--verbose   Show detailed output: full JSON, token usage, timing, cost
│   --json         Pass request as JSON (file, string, or stdin)
│   --preview      Open system preview after generation
│   --save-prompt  Save prompt as .md file
│   --http-proxy   Specify HTTP proxy
```

### Enable Tab Completion

After installation, add one of the following lines to your shell rc file:

```bash
# Bash
echo 'source <(aigc-cli completion bash)' >> ~/.bashrc

# Zsh
echo 'source <(aigc-cli completion zsh)' >> ~/.zshrc

# Fish
aigc-cli completion fish > ~/.config/fish/completions/aigc-cli.fish
```

After reloading your shell, typing `aigc-cli im+Tab` will auto-complete to `aigc-cli image`.

---

### Midjourney Subcommands

```
aigc-cli midjourney (or mj)
├── imagine       Text-to-image / Image-to-image (default entry)
├── blend         Multi-image blend (2-4 images)
├── describe      Image-to-text (reverse prompt)
├── edits         Image editing (rewrite entire image)
├── upscale       Upscale single image (U1-U4)
├── variation     Mild variation (V1-V4)
├── high-variation  Strong variation
├── low-variation   Weak variation
├── reroll        Regenerate grid
├── zoom          Zoom out / out-paint
├── pan           Pan (left/right/up/down)
├── inpaint       Local repaint entry (→ modal)
├── modal         Submit mask + prompt to complete repaint
├── video         Image-to-video
├── remix-strong  Strong remix (v8/v8.1)
├── remix-subtle  Weak remix (v8/v8.1)
└── query         Query task status
```

---

## Documentation

| Document | Content |
|---|---|
| [Installation & Configuration](docs/en/installation.md) | Install, API Key, config file, proxy |
| [Image Generation](docs/en/guide-image.md) | All parameters, sync/async modes, image-to-image, Inpainting |
| [Video Generation](docs/en/guide-video.md) | All parameters, first/last frame, reference video (APIMart) |
| [Midjourney](docs/en/guide-midjourney.md) | 17 subcommands complete guide: imagine, blend, upscale etc. |
| [AI Chat](docs/en/guide-chat.md) | Interactive multi-turn REPL, streaming, verbose stats |
| [AIGC Detection](docs/en/guide-detect.md) | Multi-signal fusion, ONNX models, FFT spectrum, emoji output |
| [Prompt Ideas](docs/en/guide-ideas.md) | Offline BM25 search engine, 10K+ prompt dataset |
| [Knowledge Base](docs/en/guide-knowledgebase.md) | Local KB: FTS5 + semantic search, vault, web search |
| [Other Commands](docs/en/guide-commands.md) | models, task, balance, dry-run, API reference |
| [API Reference](docs/en/api-reference.md) | Provider API specification sources, detection, strategy routing |
| [FAQ](docs/en/faq.md) | Install, usage, MCP, pricing FAQs |
| [MCP Integration](docs/en/guide-mcp.md) | AI agent (Claude/Cursor) integration guide |
| [Configuration Example](docs/en/config.example.yaml) | Full config file reference |

---

## Priority Rules

**CLI flags > JSON input > YAML config > Code defaults**

Proxy priority:
**`--http-proxy` flag > `HTTP_PROXY` / `HTTPS_PROXY` standard env vars**

---

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

---

## Legal Notice

Users must ensure their usage complies with applicable laws and regulations. The software authors assume no legal liability for any user actions.

## License

[MIT](LICENSE)
