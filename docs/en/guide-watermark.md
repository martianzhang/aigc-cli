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
