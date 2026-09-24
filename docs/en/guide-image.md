# Image Generation

Use `aigc-cli image` (alias `img`) to generate images.

## Basic Usage

```bash
# Simple prompt
aigc-cli image --prompt "a cat under the stars"

# With model and size
aigc-cli image --model "dall-e-3" --size "1024x1024" --prompt "a cat"
```

## Parameters

| Parameter | Description | Default |
|---|---|---|
| `--prompt` / `-p` | Image description prompt | required |
| `--model` / `-m` | Model name | `gpt-image-2-official` (APIMart) |
| `--size` | Image size: aspect ratio (`16:9`), pixels (`1024x1024`), tier (`2K`), or compound (`2K@16:9`) | varies by model |

### Provider-specific parameters

The `--size` flag accepts four forms: `<ratio>` (e.g. `16:9`), `<pixels>` (e.g. `1024x1024`), `<tier>` (e.g. `2K`), and the new compound `<tier>@<ratio>` (e.g. `2K@16:9`). The part before `@` is the resolution tier and the part after is the aspect ratio. The CLI splits it into `size` + `ratio`; anything without `@` is unchanged.

| Provider | Accepted `--size` values | Notes |
|---|---|---|
| OpenAI / generic OpenAI-compatible | `16:9`, `1024x1024` | single value: ratio or pixels |
| OpenRouter | `16:9`, `1024x1024`, `2K`, `2K@16:9` | compound maps to `size` + `aspect_ratio`; which tiers a model supports depends on that model's `supported_parameters`. Example: Recraft V4.1 only declares `aspect_ratio` (1:1/4:3/3:4/16:9/9:16/auto), so `--size 2K` returns HTTP 400 — use `--size 16:9` (ratio only) for it |
| Agnes image 2.0 | `1024x768` (pixels) | 2.0 needs pixel dimensions; see the [Agnes Image docs](https://agnes-ai.com/zh-Hans/docs/agnes-image-20-flash) |
| Agnes image 2.1 Flash | `2K@16:9` (or `--size 2K --ratio 16:9`) | 2.1 supports tiered sizing: tier + ratio |
| ModelScope | `1024x768` (pixels) | pixels only |
| APIMart | `16:9`, `1024x1024` (+ `--resolution 1k/2k/4k`) | the tier goes through `--resolution` |
| Gemini | `1024x1024` (pixels) | the CLI derives `aspect_ratio` from the pixel size |

`--ratio` is an alternative to the `@` suffix and is honored by OpenRouter (mapped to `aspect_ratio`; a ratio overrides a conflicting pixel `size`) and Agnes (mapped to `extra_body.ratio`).

> 💡 **When `--size` is combined with `--json`, the compound form is split automatically.** A `size` value containing `@` inside a `--json` body is also split. Everything else in a verbatim `--json` body stays byte-for-byte untouched.

```bash
# OpenRouter: tier + aspect ratio
aigc-cli img -P openrouter -m recraft/recraft-v4.1-flash -p prompts/4.txt --size 16:9
aigc-cli img -P openrouter -m <model> -p "a cat" --size 2K@16:9

# Agnes image 2.0: pixel dimensions
aigc-cli image --provider agnes --model agnes-image-2.0-flash \
  --size "1024x768" --prompt "a cat"
```

> **Note:** Agnes has no image upload endpoint; local `--image-url` files are automatically converted to base64 Data URIs and embedded directly — no public URL needed.

```bash
# Agnes image-to-image: local file auto-converted to a data URI
aigc-cli image --provider agnes --model agnes-image-2.5-flash \
  --size "1024x768" --image-url photo.png --prompt "a cat wearing a hat"
```

> **Note:** ModelScope has two gotchas:
>
> 1. **No image upload endpoint.** Local `--image-url` files are converted to base64 Data URIs and sent in the `image_url` field (a string for one input, an array for several).
> 2. **LoRAs must go in the `loras` field — never pass a LoRA repo ID as `--model`.** Doing so makes ModelScope **silently fall back to the base checkpoint**: no error, but the output has nothing to do with that LoRA.

### `--json` is forwarded verbatim

**What you write is what gets sent** — as long as no flags accompany `--json`. The CLI does not rename, translate, or normalize anything, so vendor parameters reach the API exactly as documented. This applies to every provider — write the provider's own shape:

When you do combine `--json` with CLI flags, only the flags you explicitly set override the matching JSON key; every key you did not touch (including vendor-only fields such as `loras`, `seed`, `steps`, or any custom key) is preserved exactly. With `--json` and no flags, the body is sent byte-for-byte unchanged.

> 💡 **JSONC is accepted.** `//` line comments, `/* */` block comments, trailing commas, and a leading UTF-8 BOM are stripped before parsing. The stripping is string-literal aware, so a `//` inside a value such as `"https://host/path"` is preserved, and a comment-free strict JSON body is unchanged byte-for-byte.

```jsonc
{
  // structural params can be commented
  "model": "krea/Krea-2-Turbo",
  "prompt": "a low-angle legwear ad",
  "loras": { "yan303145427/krea2-Cc-FY-portrait": 1.0 },
  "size": "1080x1920", // trailing comma is fine
}
```

Each provider's wire shape differs, so a verbatim `--json` body must match that provider's native shape (the CLI will not translate it). The body is sent to that provider's real endpoint:

| Provider | Actual `--json` endpoint | Native shape notes |
|---|---|---|
| ModelScope | `POST {base}/images/generations` (async task) | `loras` / `image_url` |
| APIMart | async task submit | unchanged |
| OpenRouter | `POST {base}/images` | reference images use `input_references` |
| Gemini | `POST {base}/interactions` | uses `input` / `response_format` |
| Ollama | `POST {ollama-native-base}/api/generate` | `/api/generate` body |
| ZeekAI (image-to-image) | `POST {base}/images/edits` | images use `images[].image_url` |
| Generic OpenAI-compatible (OpenAI / OpenLux / SiliconFlow / AIBaseCamp / Pollinations) and Agnes | `POST {base}/images/generations` | OpenAI images shape |

> 💡 The flag path (`--model` / `--prompt` / `--image-url` …) is unchanged — the CLI still maps and adapts fields per provider. Verbatim passthrough applies to `--json` only.

> 💡 `--dry-run` and `--verbose` print the **real endpoint and real request body**, so you can use them to confirm whether a new vendor parameter is actually wired through. For upload-based providers (such as APIMart), the preview prints one multipart upload curl per local reference image, then the generation curl with `<UPLOAD_URL_n>` placeholders.

```bash
# single LoRA (ModelScope's documented string form) + fixed seed
aigc-cli image --provider modelscope --json '{
  "model": "krea/Krea-2-Turbo",
  "prompt": "a young woman standing by a bright window, low-angle portrait",
  "loras": "yan303145427/krea2-Cc-FY-portrait",
  "seed": 12345,
  "steps": 30,
  "size": "1104x1472"
}'
```

```bash
# multiple LoRAs (ModelScope's documented id->weight map form)
aigc-cli image --provider modelscope --json '{
  "model": "krea/Krea-2-Turbo",
  "prompt": "a young woman standing by a bright window, low-angle portrait",
  "loras": {
    "yan303145427/krea2-Cc-FY-portrait": 1.0,
    "yan303145427/krea2-Cc-Ins-portrait": 1.0
  },
  "seed": 12345,
  "steps": 30,
  "size": "1104x1472"
}'
```

```bash
# image-to-image + LoRA + seed (local image paths become data URIs)
aigc-cli image --provider modelscope --json '{
  "model": "krea/Krea-2-Turbo",
  "prompt": "baimo, 3d clay white model, untextured",
  "loras": "LZFlzf10203810/3dbaimo",
  "image_urls": ["photo.jpg"],
  "seed": 12345,
  "steps": 30,
  "size": "1024x768"
}'
```

> 💡 **Weights are not normalized.** `{"a": 1.0, "b": 1.0}` is sent as 1.0/1.0 — ModelScope accepts weights that do not sum to 1, so both run at full strength. For a 7:3 blend, write `{"a": 0.7, "b": 0.3}`.

> ⚠️ A LoRA's trigger word belongs in `prompt`; without it the style or subject will not activate.

> 💡 **To check whether a parameter really reached the API:** run the same `--json` twice — the output hashes must match. If they differ, the parameter was dropped upstream.

> 💡 When a ModelScope task fails (e.g. blocked by content moderation), the error is read from the API's `errors.message`, so you see the specific reason instead of a generic `unknown error`.

### Overriding `--json` Keys with Flags

Flags and `--json` compose: each flag you explicitly set wins over the matching key, everything else in the body stays untouched. This is handy when the JSON file holds the stable structural params (LoRAs, seed, steps, size) and the prompt varies per run.

```bash
# JSON holds the structural params; the prompt comes from a separate flag
aigc-cli image --provider modelscope --json downloads/prompt.json --prompt "a low-angle legwear ad"
```

In this example the JSON's `prompt` is replaced by the flag value, while `loras`, `seed`, `steps`, and `size` from the JSON are preserved exactly. Flag-to-key mapping follows the normal flag path (`--output-format` → `output_format`, `--image-url` → `image_urls`). Behavioral flags such as `--dry-run`, `--preview`, `--provider`, `--api-key`, `--api-base`, `--http-proxy`, `--output`, `--verbose`, and `--timeout` never enter the body.

## How Reference Images Are Sent: Upload vs Inline Data URI

How a local reference image (`--image-url`, `--mask-url`, or an image field pointing at a local file inside `--json`) is sent depends on the provider: either it is uploaded to an upload endpoint first and the returned URL is referenced by the generation request, or it is encoded inline as a `data:image/<mime>;base64,...` value in the body. Remote `https://` URLs and data URIs always pass through unchanged, never re-encoded or re-uploaded.

| Handling | Provider | Image value in the preview |
|---|---|---|
| Upload (send file, then reference its URL) | APIMart | `<UPLOAD_URL_0>`, `<UPLOAD_URL_1>`, … |
| Inline data URI (no upload endpoint) | OpenRouter, Gemini, ModelScope, Zeekai (image-to-image), Agnes | `data:image/png;base64,...` |
| Inline data URI (default OpenAI-compatible) | OpenAI / OpenLux / SiliconFlow and other generic relays | `data:image/png;base64,...` inside the `image_urls` array |

### Upload-based (APIMart): `--dry-run` prints upload curls + the generation curl

APIMart cannot take a local file directly in the generation body: the CLI uploads **each local reference image** with a multipart request first, then sends the generation request with the returned public URL. `--dry-run` prints exactly that sequence: N local images produce N upload curls, followed by one generation curl whose image value is the placeholder `<UPLOAD_URL_n>`.

```bash
aigc-cli image --base-url "https://api.apimart.ai" \
  --prompt "turn this photo into a Ghibli-style scene" \
  --image-url ./photo.png --dry-run
```

```bash
# Output (API key masked)
curl -X POST https://api.apimart.ai/v1/uploads/images \
  -H "Authorization: Bearer ...xxxx" \
  -F "file=@./photo.png"
curl -X POST https://api.apimart.ai/v1/images/generations \
  -H "Authorization: Bearer ...xxxx" \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-image-2-official","prompt":"turn this photo into a Ghibli-style scene","image_urls":["<UPLOAD_URL_0>"]}'
```

`--dry-run` is preview only: **it makes no network calls and does not upload**. On a real run the placeholder is replaced with the URL returned by the upload. Remote URLs do not trigger an upload and stay in the body as-is. `--mode async` also selects this upload plan.

> 💡 **`--json` uploads too:** image fields pointing at local files in a verbatim `--json` body are replaced with placeholders, and the uploaded URLs are written back on a real run. The `--dry-run` body keeps the original `--json` shape, only the image values become placeholders.

### Inline providers (no upload endpoint)

These providers encode local files as data URIs inline in the body; `--dry-run` / `--verbose` show each provider's real body shape:

| Provider | Local reference lands in | Shape in the preview body |
|---|---|---|
| OpenRouter | `input_references[]` | `{"type":"image_url","image_url":{"url":"data:image/png;base64,..."}}` |
| Gemini | `input[]` | `{"type":"image","data":"<base64>","mime_type":"image/png"}` |
| ModelScope | `image_url` | a string for one image, an array for several, each `data:image/png;base64,...` |
| Zeekai (image-to-image) | `images[].image_url` | `"images":[{"image_url":"data:image/png;base64,..."}]`, with a trailing `# note:` line |
| Agnes | `extra_body.image` | `"extra_body":{"image":["data:image/png;base64,..."]}` |
| Default OpenAI-compatible | `image_urls[]` | `"image_urls":["data:image/png;base64,..."]` |

> 💡 **Gemini image-to-image now works:** local and remote reference images are both sent as image items in `input` (a local file becomes `{"type":"image","data":...,"mime_type":...}`, a remote URL becomes `{"type":"image","uri":...}`), instead of being dropped.

> ⚠️ **Native OpenAI `/images/generations` has no image field:** the CLI encodes local reference images into `image_urls` before sending, which works for compatible relays that accept that field (a self-contained data URI is strictly better than sending a local path that never worked). Native OpenAI image editing uses `POST /v1/images/edits`, so use an `/images/edits` relay (such as Zeekai) or the provider's own edit protocol.

## Sync Mode

The default mode for OpenAI-compatible APIs. Returns the image URL directly after generation.

```bash
aigc-cli image --prompt "a cat" --mode sync
```

## Async Mode

Used by APIMart and similar providers. Submits a task and polls until completion.

```bash
aigc-cli image --prompt "a cat" --mode async --timeout 120
```

## Transparent Background (OpenAI gpt-image-1)

`--background transparent` generates images with an alpha channel. Requires `--output-format png` or `webp` (`jpeg` does not support transparency):

```bash
aigc-cli image --model "gpt-image-1" \
  --prompt "a logo for a coffee brand, isolated subject" \
  --background transparent --output-format png
```

Note: dall-e-2/3 do not support `--background`.

## Image-to-Image

Generate variations based on an existing image:

```bash
aigc-cli image --prompt "a cat wearing a hat" --image-url /path/to/input.png
```

URLs and data URIs are also supported:

```bash
aigc-cli image --prompt "edit this" --image-url "https://example.com/image.png"
aigc-cli image --prompt "edit this" --image-url "data:image/png;base64,..."
```

> 💡 How each provider sends local files (an upload curl vs an inline data URI) is covered in [How Reference Images Are Sent](#how-reference-images-are-sent-upload-vs-inline-data-uri).

### Image-to-Image on `/images/edits` Relays (ZeekAI auto-detected, no config)

ZeekAI (`zeekai.cc`) rejects `image_urls` on `/images/generations` (400) and instead exposes `POST /images/edits` with a body like `{"model","prompt","size","images":[{"image_url":"..."}]}`. aigc-cli **auto-detects the ZeekAI domain and switches to that protocol — no configuration or flag needed**:

```bash
# Local files are embedded as data:image/png;base64,... in images[].image_url
aigc-cli image --provider zeekai -i photo.png -p "replace the background with a starry sky"
```

This protocol does not support `--mask-url` (it fails with a clear error). When ZeekAI is not detected, behavior is unchanged: top-level `image_urls` to `/images/generations`.

## Decode Mode (--decode)

`--decode` converts base64 text files (data URI or raw base64) into inline
data URIs before sending the request — provider-agnostic, works with any
OpenAI-compatible API (real image files and remote URLs pass through to the
existing upload path unchanged). Without `--prompt`, it runs purely locally —
decode/convert files and save them to the output dir with no API call:

```bash
# Decode a base64 text file to a real image (local, no API call)
aigc-cli image --decode --image-url image.txt

# Also convert the format while decoding
aigc-cli image --decode --output-format png --image-url image.txt

# Convert a real image's format (jpg → png)
aigc-cli image --decode --output-format png --image-url photo.jpg

# Decode before using a reference image (edit mode)
aigc-cli image --edit --decode --image-url image.txt --prompt "Make it cinematic"

# Decode works with any provider (converted to inline data URI):
aigc-cli image --decode --image-url image.txt --prompt "Keep this style"
```

Supported target formats: `png`, `jpg`/`jpeg`, `webp`, `avif`, `jxl` (via
`--output-format`). Text formats are also supported:
- `base64` — pure base64 text (no prefix; `base64 -d` restores the image)
- `datauri` — a data URI (`data:<mime>;base64,...`), ready to paste into
  markdown, HTML, or APIs

```bash
# Convert an image to pure base64 text
aigc-cli image --decode --output-format base64 --image-url photo.jpg

# Convert an image to a data URI (same form as image.txt)
aigc-cli image --decode --output-format datauri --image-url photo.jpg
```

Decode-only mode (no `--output-format`) keeps the detected
format from the data URI MIME type or magic bytes.

## Inpainting

Replace masked areas of an image:

```bash
aigc-cli image --prompt "a dog" --image-url input.png --mask-url mask.png
```

## OpenRouter Dedicated API

When using OpenRouter, aigc-cli automatically routes to the dedicated image API:

```bash
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
export OPENAI_API_KEY="sk-or-xxx"
aigc-cli image --model "openai/gpt-image-2" --prompt "a cat"
```

## Dry-Run

Preview the API request without sending:

```bash
aigc-cli image --prompt "a cat" --dry-run
```

For upload-based providers (APIMart), the preview also prints the multipart upload curl for each local reference image, followed by the generation curl. See [How Reference Images Are Sent](#how-reference-images-are-sent-upload-vs-inline-data-uri) for a full example.

## JSON Input

Pass the full request as JSON:

```bash
aigc-cli image --json '{"prompt": "a cat", "model": "dall-e-3", "n": 1}'
```

Or from a file:

```bash
aigc-cli image --json @request.json
```
