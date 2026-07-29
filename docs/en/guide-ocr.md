# OCR

Offline text recognition using DBNet + CRNN with ONNX runtime. All processing is local — no API Key needed.

## Commands

```
aigc-cli ocr
├── init    Download OCR models
└── scan    Recognize text in images
```

## Usage

```bash
# Download models first
aigc-cli ocr init

# Recognize text
aigc-cli ocr scan photo.png

# With verbose output
aigc-cli ocr scan photo.png --verbose
```

## How It Works

1. **Detection** (DBNet): Detects text regions in the image
2. **Recognition** (CRNN): Recognizes characters in each region
3. **Post-processing**: CTC decoding, word splitting

Supports Chinese and English text recognition.

## Online Fallback

If a provider is configured, OCR can use an online LLM:

```yaml
defaults:
  ocr:
    provider: "my-openrouter"
    model: "gpt-4o"
```

Without provider config, it defaults to local ONNX.
