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

### Document parsing (SoMark / OXR)

For PDFs and full pages you want **layout, tables, formulas and reading
order**, not line-by-line text. Two providers call SoMark's document-parse API,
which does layout analysis server-side and returns Markdown with heading
levels, Markdown tables and LaTeX formulas.

Configure one provider with `type: somark`. Hosted SoMark, the free OXR
endpoint and a self-hosted OXR all speak the same protocol — only `base_url`
differs:

```yaml
providers:
  # Hosted SoMark: 200MB / 300 pages / QPS 4
  somark:
    type: somark
    api_key: "sk-xxx"
    base_url: "https://somark.ai/api/v1"

  # Free OXR: no key (10MB / 20 pages, queued)
  free-oxr:
    type: somark
    base_url: "https://free-oxr.somark.ai/api/v1"

  # Self-hosted OXR: official Docker image at http://<host>:8000/v1
  oxr-local:
    type: somark
    base_url: "http://gpu-box:8000/v1"

defaults:
  ocr:
    provider: "free-oxr"     # or somark / oxr-local
```

With `type: somark` and no `base_url`, the hosted endpoint
`https://somark.ai/api/v1` is used.

```bash
aigc-cli ocr scan paper.pdf                        # uses defaults.ocr.provider
aigc-cli ocr scan paper.pdf --provider somark      # switch to the hosted API
aigc-cli ocr scan paper.pdf --provider somark --inject-json '{"feature_config":{"enable_title_level_recognition":true}}'
```

#### Image localization (on by default)

SoMark deletes parse-time image resources after **30 days**, so the image URLs
in the result expire. After a successful parse, `ocr scan` therefore:

1. downloads every remote image in the Markdown (`![](https://…)` and HTML
   `<img src="https://…">`);
2. stores them in `<name>.assets/` next to the Markdown (`image_001.png`,
   `image_002.jpg`, …, each URL fetched once);
3. rewrites the links to relative paths (`![](paper.assets/image_001.png)`).

```
paper.pdf
paper.md
paper.assets/
├── image_001.png
└── image_002.png
```

- A failed download (404, non-image response, >64MB, …) never aborts the
  parse: that link keeps the remote URL and a `⚠ image not saved` line is
  printed.
- `-o -` (stdout) does not download images; remote URLs are kept and a notice
  is printed.
- Alternatively let the server bundle everything: a ZIP export contains the
  Markdown and its images.

```bash
# ZIP export: output_formats must contain zip and image must be file
aigc-cli ocr scan paper.pdf --provider somark --json '{
  "output_formats":["zip"],
  "element_formats":{"image":"file"}
}'
# → saves paper.zip (no separate paper.md)
```

> `zip` can only be used alone or with `json`, never together with `markdown`.
> Since `--inject-json` appends arrays instead of replacing them, request ZIP
> with `--json` to replace the default body wholesale.

#### Request parameters (`--json` / `--inject-json`)

The parser's API parameters (`output_formats`, `element_formats`,
`feature_config`, `extra`) go through the project-wide `--json` (whole request
body) and `--inject-json` (additive merge) flags — no provider-specific flags
are added. Both accept an inline JSON string or a path to a JSON file; neither
reads stdin.

```bash
# Heading levels + cross-page merging (hosted SoMark; free OXR ignores them
# and reports that in warnings)
aigc-cli ocr scan paper.pdf --provider somark \
  --inject-json '{"feature_config":{"enable_title_level_recognition":true,"enable_text_cross_page":true,"enable_table_cross_page":true}}'

# Full request body: Markdown only, HTML tables, MathML formulas, inline images
aigc-cli ocr scan paper.pdf --provider somark --json '{
  "output_formats":["markdown"],
  "element_formats":{"image":"base64","formula":"mathml","table":"html"}
}'

# Free OXR sampling parameters
aigc-cli ocr scan paper.pdf --provider free-oxr --inject-json '{"extra":{"temperature":0.2}}'
```

Default body when `--json` is omitted:

```json
{"output_formats":["markdown","json"],"element_formats":{"image":"url","formula":"latex","table":"markdown"}}
```

> `file`, `file_url` and `api_key` are generated by the CLI; same-named keys in
> JSON are ignored. `--inject-json` merges like everywhere else: objects
> recurse, arrays append, scalars overwrite.

> `--prompt`, `--model`, `--pages`, `--engine`, `--lang` and `--spellcheck` do
> nothing on this path: the parser takes no prompt or model and always uploads
> the whole file (the server splits pages itself). Explicitly passing one prints
> a `⚠` warning. Split the PDF locally first if you need page ranges.

| Model | Task | Prompt |
|---|---|---|
| **minicpm-ocr** | Text | `Text Recognition:` |
| **minicpm-ocr** | Table (OTSL output) | `Table Recognition:` |
| **minicpm-ocr** | Formula (LaTeX output) | `Formula Recognition:` |
