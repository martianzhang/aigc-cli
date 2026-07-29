# Background Removal

Use `aigc-cli background` (alias `bg`) to remove image backgrounds using RMBG 2.0 semantic segmentation. All processing is offline ONNX — no API Key needed.

## Usage

```bash
# Remove background from an image
aigc-cli background input.png

# Specify output path
aigc-cli background input.png --output output.png
```

## How It Works

1. **Preprocessing**: Resize and normalize the input image
2. **Inference**: RMBG 2.0 ONNX model predicts the alpha mask
3. **Post-processing**: Apply mask, output RGBA image

## Model

The default model is `rmbg-2.0`. Model files are stored in `~/.config/aigc-cli/models/`.

## Online Fallback

If a provider is configured, background removal can use an online API:

```yaml
defaults:
  background:
    provider: "my-apimart"
```

Without provider config, it defaults to local ONNX.
