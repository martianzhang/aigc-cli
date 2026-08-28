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
| `--quality` | Quality: `standard` or `hd` | `standard` |
| `--style` | Style: `vivid` or `natural` | `vivid` |
| `--output-format` | Output format: `png`, `jpg`, `webp`, `avif`, `jxl` | `png` |
| `--background` | Background: `auto`, `opaque`, or `transparent` (transparent requires `png`/`webp` output) | `auto` |
| `--n` | Number of images to generate | `1` |
| `--image-url` | Input image for image-to-image or editing | — |
| `--decode` / `-d` | Decode base64 text files (data URI / raw base64) in `--image-url` to real images; combine with `--output-format` to convert format | `false` |
| `--mask-url` | Mask image for inpainting | — |
| `--mode` | Mode: `sync` or `async` | auto-detected |

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
