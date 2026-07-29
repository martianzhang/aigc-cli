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

# With watermark removal
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

## Watermark Learning

Before removing watermarks, you must learn the watermark pattern:

```bash
# 1. Generate pure black and gray images with watermark enabled
# 2. Place them in ~/.config/aigc-cli/watermark/
# 3. Learn the watermark
aigc-cli detect --learn-watermark <producer-name>
```

See [guide-watermark.md](guide-watermark.md) for details.

## Config

```yaml
detect:
  models_dir: "~/.config/aigc-cli/models"
  model: "vit-base"  # or "distilled-vit"
```
