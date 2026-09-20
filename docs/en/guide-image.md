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
| `--size` | Image size (`1024x1024`, `1792x1024`, etc.) | varies by model |

> **Note:** The `--size` format varies by provider. Most providers (OpenAI, OpenRouter, APIMart) accept aspect ratios like `1:1`, `16:9`. However, **Agnes requires pixel dimensions** (e.g. `1024x1024`, `1024x768`, `768x1024`). See the [Agnes Image docs](https://agnes-ai.com/zh-Hans/docs/agnes-image-20-flash) for details.

### Provider-specific parameters

Some providers require non-standard parameter formats. aigc-cli handles these automatically where possible, but you should be aware of them when specifying `--size`:

- **Agnes** (`agnes-image-2.0-flash`, `agnes-image-2.5-flash`): requires **pixel dimensions** for `--size`, e.g. `1024x1024`, `1024x768`, `768x1024`. Aspect ratios like `16:9` are not documented. See the [Agnes Image docs](https://agnes-ai.com/zh-Hans/docs/agnes-image-20-flash).

```bash
# Agnes: use pixel dimensions for --size
aigc-cli image --provider agnes --model agnes-image-2.5-flash \
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

**What you write is what gets sent.** The CLI does not rename, translate, or normalize anything, so vendor parameters reach the API exactly as documented. This applies to every provider — write the provider's own shape:

Each provider's wire shape differs, so a verbatim `--json` body must match that provider's native shape (the CLI will not translate it). The body is sent to that provider's real endpoint:

| Provider | Actual `--json` endpoint | Native shape notes |
|---|---|---|
| ModelScope | `POST {base}/images/generations` (async task) | `loras` / `image_url` |
| APIMart | async task submit | unchanged |
| OpenRouter | `POST {base}/images` | reference images use `input_references` |
| Gemini | `POST {base}/interactions` | uses `input` / `response_format` |
| Ollama | `POST {ollama-native-base}/api/generate` | `/api/generate` body |
| ZeekAI (image-to-image) | `POST {base}/images/edits` | images use `images[].image_url` |
| Generic OpenAI-compatible (OpenAI / Yunwu / SiliconFlow / AIBaseCamp / Pollinations) and Agnes | `POST {base}/images/generations` | OpenAI images shape |

> 💡 The flag path (`--model` / `--prompt` / `--image-url` …) is unchanged — the CLI still maps and adapts fields per provider. Verbatim passthrough applies to `--json` only.

> 💡 `--dry-run` and `--verbose` print the **real endpoint and real request body**, so you can use them to confirm whether a new vendor parameter is actually wired through.

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

## JSON Input

Pass the full request as JSON:

```bash
aigc-cli image --json '{"prompt": "a cat", "model": "dall-e-3", "n": 1}'
```

Or from a file:

```bash
aigc-cli image --json @request.json
```
