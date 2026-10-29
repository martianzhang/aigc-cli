# API Reference

This document lists the API specification sources used by aigc-cli's provider detection and strategy routing.

## Provider API Sources

| Provider | Base URL | Image API | Video API | Music API |
|---|---|---|---|---|
| OpenAI | `https://api.openai.com/v1` | `POST /v1/images/generations` (text) / `POST /v1/images/edits` (image-to-image, multipart) | — | — |
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
| `xiaomimimo.com`, `mimo.mi.com` | MiMo (chat reports `web_search_usage`) |
| `openai.com`, `openai.azure.com`, `ofox.ai`, `ofox.run`, `ofox.io` | OpenAI-family (image input auto-routes to multipart `POST /images/edits`) |
| `openlux.ai` (in URL) | OpenLux |
| `somark.ai`, `somark.cn`, `somark.tech` | SoMark / OXR document parse (`POST /parse/sync`, not chat) |
| `localhost` or `127.0.0.1` | Local (no API Key) |

## Chat Protocol

By default `chat` uses `POST /v1/chat/completions` (provider type `openai` or its explicit alias `openai_chat_completions`). A provider can select the OpenAI Responses protocol with `type: openai_responses`, which routes `chat` to `POST {base_url}/responses`. Use this when a reasoning model rejects function tools on the chat-completions endpoint (HTTP 400 about `reasoning_effort`). Only `chat` is affected.

## SoMark / OXR Document Parsing

SoMark and the open-source OXR share one **document-parse protocol**, not chat completions. `ocr scan` uploads the whole PDF/image and gets Markdown plus structured JSON back.

| Endpoint | Purpose |
|---|---|
| `POST /parse/sync` | Synchronous parse (used by the CLI) |
| `POST /parse/async` + `/parse/async_check` | Async parse (not used) |
| `POST /convert/sync`, `POST /convert/async(_check)` | PDF → Word/PPT (not used; unsupported by OXR) |
| `POST /usage` | Remaining quota (used by `aigc-cli balance`; unsupported by OXR) |

Auth is a **form field** `api_key=sk-***`, not `Authorization: Bearer`.

| Endpoint | Base URL | Auth | Limits |
|---|---|---|---|
| Hosted SoMark | `https://somark.ai/api/v1` | `api_key` required | 200MB / 300 pages / QPS 4 |
| Free OXR | `https://free-oxr.somark.ai/api/v1` | not checked (`dummy`) | 10MB / 20 pages, queued |
| Self-hosted OXR | `http://<host>:8000/v1` | not checked | none |

Key request fields: `file` (PDF/common images; OXR rejects Office, gif/heic), `output_formats` (`markdown` / `json`), `element_formats` (`{"image":"url|base64|none","formula":"latex","table":"markdown|html"}`), `feature_config` (`keep_header_footer` works everywhere; `enable_title_level_recognition`, `enable_text_cross_page`, `enable_table_cross_page`, `enable_inline_image`, `enable_image_ocr` are hosted-only), and `extra` (OXR-only sampling: `temperature`, `top_p`, `repetition_penalty`).

Images: parse-time image URLs **expire after 30 days**. The CLI downloads them
into `<name>.assets/` and rewrites the Markdown links
(`service.LocalizeMarkdownImages`); you can also request a ZIP instead
(`output_formats` with `zip` and `element_formats.image=file`; zip combines only
with json).

Errors also come back as HTTP 200 with `code != 0` (e.g. `{"code":1107,"message":"Invalid API key.","data":""}`). OXR ignores unsupported `enable_*` flags and lists them in `warnings` instead of failing.

Implementation: `internal/provider/somark.go` (`ParseDocument`, `SomarkParseURL`); routing via `provider.IsDocumentProvider` (explicit `type: somark` or a SoMark host). Select it with `--provider <name>` or `defaults.ocr.provider`; its API parameters are passed with the shared `--json` / `--inject-json` flags (see the [OCR guide](guide-ocr.md)).

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
| deepseek | `api.deepseek.com` (`POST /v1/tools/search`) | $0.75/1000 successful requests (≈¥5/1000) |
| exa | `api.exa.ai` (`POST /search`, `POST /contents`) | $7/1000 auto searches, $1/1000 contents pages ($10 free credit/month) |
| wsa | `api.wsa.cloud.tencent.com` (`POST /SearchPro`, API KEY mode) | Tencent Yuanbao Search; see the [product docs](https://cloud.tencent.com/document/product/1806/121799) |

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
