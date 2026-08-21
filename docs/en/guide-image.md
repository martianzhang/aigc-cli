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
