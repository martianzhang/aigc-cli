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

### Ollama OCR models

Named providers with `type: ollama` are called through the native `/api/chat`
endpoint. Any multimodal model works. Request-driven OCR models (`minicpm-ocr`
and friends) need their prompt wording passed explicitly with `--prompt` — the
generic default is meant for general vision models and can make them describe
the image or invent layout HTML.

```yaml
providers:
  ollama:
    type: ollama
    base_url: "http://localhost:11434/v1"

defaults:
  ocr:
    provider: "ollama"
    model: "minicpm-ocr"
```

```bash
aigc-cli ocr scan page.png --provider ollama --model minicpm-ocr -p "Text Recognition:"
aigc-cli ocr scan table.png --provider ollama -m minicpm-ocr -p "Table Recognition:"
```

`--model` also accepts the `-m` shorthand.

By default the result is saved as a `.md` file next to the input. Pass `-o -` to
print it to stdout instead and write no file:

```bash
aigc-cli ocr scan page.png -o - | pbcopy
```

| Model | Task | Prompt |
|---|---|---|
| **minicpm-ocr** | Text | `Text Recognition:` |
| **minicpm-ocr** | Table (OTSL output) | `Table Recognition:` |
| **minicpm-ocr** | Formula (LaTeX output) | `Formula Recognition:` |
