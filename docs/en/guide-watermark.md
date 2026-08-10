# Watermark Engine

aigc-cli supports learning and removing custom watermarks. No vendor watermark data is included — all watermarks are user-generated.

## Workflow: Two-Shot Watermark Learning

### Step 1: Generate Pure Color Images

Generate two solid-color images with the AI platform's watermark enabled:

| File | Color | Prompt |
|---|---|---|
| `<name>.black.png` | RGB(0,0,0) | "Generate a pure black image, RGB(0,0,0), no content. 1:1" |
| `<name>.gray.png` | RGB(128,128,128) | "Generate a pure gray image, RGB(128,128,128), no content. 1:1" |

### Step 2: Place in Watermark Directory

```bash
cp <name>.black.png <name>.gray.png ~/.config/aigc-cli/watermark/
```

### Step 3: Learn

```bash
aigc-cli detect --learn-watermark <name>
```

Output: `~/.config/aigc-cli/watermark/<name>.watermark.png` (grayscale + PNG tEXt metadata)

### Step 4: Remove

```bash
aigc-cli detect photo.png --remove-watermark --producer <name>
```

## Reference Seeds

The `scripts/assets/` directory contains reference seed images for testing:

- `baidu.black.png`, `baidu.gray.png`
- `doubao.black.png`, `doubao.gray.png`

## Verification Scripts

```bash
# Visual diff + heatmap
python scripts/check_watermark.py <original> <cleaned> --report

# Quantitative verification (PASS/WARN/FAIL)
python scripts/verify_watermark.py <original> <cleaned>
```

## Reference & Recommended Projects

**Reference projects (algorithm source)**

- **[gemini-watermark-remover](https://github.com/GargantuaX/gemini-watermark-remover)** — Gemini sparkle alpha map data and size catalog (reference for this CLI's MI-GAN / Alpha Map removal)
- **[remove-ai-watermarks](https://github.com/wiltodelta/remove-ai-watermarks)** — Text watermark alpha map assets and two-capture extraction algorithm

**Recommended third-party tools (better removal quality)**

If the built-in algorithms don't meet your needs, try these third-party tools:

- **[Pilio Image Watermark Remover](https://pilio.ai/image-watermark-remover)** — Online AI watermark removal (server-side inpainting) that repairs covered faces/textures/text. Handles arbitrary watermark types (logos, signatures, tiled marks, date stamps). Free to try
- **[Gemini Watermark Remover (Chrome extension)](https://chromewebstore.google.com/detail/cjlmnfcfnofnglkphbcdclbpimdjkmdf)** — Same source as the reference project (by Pilio); auto-removes watermarks on Gemini pages
- **[Gemini Watermark Remover (CLI/local)](https://github.com/GargantuaX/gemini-watermark-remover)** — The same project's Node CLI (`gwr remove <input> --output <file>`), runs locally with no upload

> ⚠️ Before using any watermark removal tool, make sure you own the image or have permission to edit it.
