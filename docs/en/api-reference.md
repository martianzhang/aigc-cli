# API Reference

This document lists the API specification sources used by aigc-cli's provider detection and strategy routing.

## Provider API Sources

| Provider | Base URL | Image API | Video API | Music API |
|---|---|---|---|---|
| OpenAI | `https://api.openai.com/v1` | `POST /v1/images/generations` | — | — |
| OpenRouter | `https://openrouter.ai/api/v1` | `POST /api/v1/images` | `POST /api/v1/videos` | `POST /api/v1/chat/completions` (Lyria-3, sync streaming) |
| APIMart | `https://api.apimart.ai` | Async task | Async task + VEO3 Remix | `POST /v1/music/generations` (async, suno/flowmusic via `model`) |
| ZeekAI | detector | `POST /v1/images/generations` (text) / `POST /v1/images/edits` (image-to-image) | — | — |
| OpenLux | https://api.openlux.ai/v1 | — | POST /v1/video/create + GET /v1/video/query | — |

## Detection Logic

Provider detection uses `base_url` pattern matching:

| Pattern | Provider |
|---|---|
| `openrouter.ai` | OpenRouter |
| `apimart.ai` | APIMart |
| `zeekai.cc` | ZeekAI (image-to-image auto-routes to `/images/edits`) |
| `openlux.ai` (in URL) | OpenLux |
| `localhost` or `127.0.0.1` | Local (no API Key) |

## Chat Protocol

By default `chat` uses `POST /v1/chat/completions` (provider type `openai` or its explicit alias `openai_chat_completions`). A provider can select the OpenAI Responses protocol with `type: openai_responses`, which routes `chat` to `POST {base_url}/responses`. Use this when a reasoning model rejects function tools on the chat-completions endpoint (HTTP 400 about `reasoning_effort`). Only `chat` is affected.

## Strategy Routing

aigc-cli uses a `match-run` dispatch pattern:

```
imageStrategies:
  - match: sync mode, OpenAI-compatible
    run: POST /v1/images/generations
  - match: OpenRouter detected
    run: POST /api/v1/images (dedicated API)
  - match: APIMart detected
    run: Async task submit → poll → download
```

`music` does not use a table; it dispatches directly on the detected provider: OpenRouter → sync streaming `POST /v1/chat/completions` (`modalities: ["text","audio"]`, `stream: true`), otherwise APIMart async `POST /v1/music/generations` → `GET /v1/music/tasks/{task_id}`.

## Web Search Providers

| Provider | API | Pricing |
|---|---|---|
| duckduckgo | HTML scrape | Free |
| brave | `api.search.brave.com` | Paid (free tier available) |
| firecrawl | `api.firecrawl.dev` | Paid (free tier available) |
| doubao | `open.feedcoopapi.com` | 0.020 CNY/request (500 free/month) |

## Midjourney API

aigc-cli translates Midjourney subcommands to the provider's API format. Currently supports APIMart Midjourney API.

## Music API

| Provider | Endpoint | Mode |
|---|---|---|
| APIMart | `POST /v1/music/generations` → `GET /v1/music/tasks/{task_id}` | Async; `model` selects `suno` (default) or `flowmusic` |
| OpenRouter | `POST /api/v1/chat/completions` (`modalities: ["text","audio"]`, `stream: true`) | Sync streaming; Google Lyria-3 (`google/lyria-3-clip-preview` / `-pro-preview`) |

## AIGC Detection Signals

| Signal | Standard | Method |
|---|---|---|
| C2PA | Adobe Content Credentials | XMP metadata parsing |
| TC260 | GB 45438-2025 | PNG tEXt / JPEG COM |
| SynthID | Google DeepMind | Metadata inference |
| ONNX | — | Vision transformer classification |
| FFT | — | Frequency domain analysis |
| SRM | — | Noise residual analysis |
| JPEG | — | Quantization table analysis |

## Watermark Engine

The watermark engine uses a differential approach: learns the watermark pattern from pure-color images and subtracts it from target images.
