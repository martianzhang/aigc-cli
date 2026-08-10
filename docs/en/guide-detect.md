# AIGC Detection

Use `aigc-cli detect` to detect watermarks, metadata, and AI-generated content using multi-signal fusion.

## Signals

| Signal | Description |
|---|---|
| C2PA | Content Credentials (Adobe provenance) |
| TC260 | Chinese national standard GB 45438-2025 |
| SynthID | Google DeepMind invisible watermark |
| ONNX | AI classification model inference |
| FFT | Frequency spectrum analysis |
| SRM Noise | Noise residual analysis |
| JPEG Quantization | JPEG quantization table analysis |
| Visible Watermark | Visible watermark detection |

## Usage

```bash
# Basic detection
aigc-cli detect photo.png

# Detailed output
aigc-cli detect photo.png --detail

# Crop to remove watermarks (no learning required)
aigc-cli detect photo.png --crop-watermark auto      # auto-detect and crop
aigc-cli detect photo.png --crop-watermark 97%       # keep 97% of edge length
aigc-cli detect photo.png --crop-watermark 1920x1080 # crop to target size

# Remove watermarks with AI inpainting
aigc-cli detect photo.png --remove-watermark --producer doubao
```

## Output

Detection results are shown with emoji indicators:

```
🔍 C2PA: not found
🔍 TC260: AI-generated (doubao)
🔍 SynthID: not found
🔍 ONNX: 92.3% AI
🔍 FFT: artificial pattern detected
🔍 SRM: synthetic texture
🔍 JPEG: non-standard tables
```

## Watermark Removal

Three methods are available:

| Method | Description |
|---|---|
| **Crop** | Generic method - detects watermark location and crops it out. No learning required |
| **MI-GAN** | AI inpainting using ONNX Runtime (Picsart MI-GAN, ICCV 2023). Requires `migan.onnx` model |
| **Alpha Map** | Classical reverse alpha blending. Requires learned watermark config |

### Crop-based Removal

```bash
# Auto mode: detect watermark and crop, or apply 5% margin if not detected
aigc-cli detect photo.png --crop-watermark auto

# Keep percentage of edge length (centered crop)
aigc-cli detect photo.png --crop-watermark 97%

# Crop to target dimensions (centered)
aigc-cli detect photo.png --crop-watermark 1920x1080
```

### Watermark Learning

Before removing watermarks with Alpha Map method, you must learn the watermark pattern:

```bash
# 1. Generate pure black and gray images with watermark enabled
# 2. Place them in ~/.config/aigc-cli/watermark/
# 3. Learn the watermark
aigc-cli detect --learn-watermark <producer-name>
```

See [guide-watermark.md](guide-watermark.md) for details.

## Reference & Recommended Projects

**Reference projects (algorithm source)**

- **[gemini-watermark-remover](https://github.com/GargantuaX/gemini-watermark-remover)** — Gemini sparkle alpha map data and size catalog (reference for this CLI's MI-GAN / Alpha Map removal)
- **[remove-ai-watermarks](https://github.com/wiltodelta/remove-ai-watermarks)** — Doubao/Jimeng text watermark alpha map assets and scoring parameters

**Recommended third-party tools (better removal quality)**

If the built-in algorithms don't meet your needs, try these third-party tools:

- **[Pilio Image Watermark Remover](https://pilio.ai/image-watermark-remover)** — Online AI watermark removal (server-side inpainting) that repairs covered faces/textures/text. Handles arbitrary watermark types (logos, signatures, tiled marks, date stamps). Free to try
- **[Gemini Watermark Remover (Chrome extension)](https://chromewebstore.google.com/detail/cjlmnfcfnofnglkphbcdclbpimdjkmdf)** — Same source as the reference project (by Pilio); auto-removes watermarks on Gemini pages
- **[Gemini Watermark Remover (CLI/local)](https://github.com/GargantuaX/gemini-watermark-remover)** — The same project's Node CLI (`gwr remove <input> --output <file>`), runs locally with no upload

> ⚠️ Before using any watermark removal tool, make sure you own the image or have permission to edit it.

## Config

```yaml
detect:
  models_dir: "~/.config/aigc-cli/models"
  model: "vit-base"  # or "distilled-vit"
```
