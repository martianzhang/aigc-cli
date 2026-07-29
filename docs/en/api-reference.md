# API Reference

This document lists the API specification sources used by aigc-cli's provider detection and strategy routing.

## Provider API Sources

| Provider | Base URL | Image API | Video API |
|---|---|---|---|
| OpenAI | `https://api.openai.com/v1` | `POST /v1/images/generations` | — |
| OpenRouter | `https://openrouter.ai/api/v1` | `POST /api/v1/images` | `POST /api/v1/videos` |
| APIMart | `https://api.apimart.ai` | Async task | Async task + VEO3 Remix |
| Yunwu AI | detector | — | `POST /v1/video/create` |

## Detection Logic

Provider detection uses `base_url` pattern matching:

| Pattern | Provider |
|---|---|
| `openrouter.ai` | OpenRouter |
| `apimart.ai` | APIMart |
| `yunwu` (in URL) | Yunwu AI |
| `localhost` or `127.0.0.1` | Local (no API Key) |

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

## Web Search Providers

| Provider | API | Pricing |
|---|---|---|
| duckduckgo | HTML scrape | Free |
| brave | `api.search.brave.com` | Paid (free tier available) |
| firecrawl | `api.firecrawl.dev` | Paid (free tier available) |
| doubao | `open.feedcoopapi.com` | 0.020 CNY/request (500 free/month) |

## Midjourney API

aigc-cli translates Midjourney subcommands to the provider's API format. Currently supports APIMart Midjourney API.

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
