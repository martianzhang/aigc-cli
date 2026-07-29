# Midjourney

Use `aigc-cli midjourney` (alias `mj`) to interact with Midjourney's API without Discord.

## 17 Subcommands

```
aigc-cli midjourney (or mj)
├── imagine       Text-to-image / Image-to-image (default entry)
├── blend         Multi-image blend (2-4 images)
├── describe      Image-to-text (reverse prompt)
├── edits         Image editing (rewrite entire image)
├── upscale       Upscale single image (U1-U4)
├── variation     Mild variation (V1-V4)
├── high-variation  Strong variation
├── low-variation   Weak variation
├── reroll        Regenerate grid
├── zoom          Zoom out / out-paint
├── pan           Pan (left/right/up/down)
├── inpaint       Local repaint entry (→ modal)
├── modal         Submit mask + prompt to complete repaint
├── video         Image-to-video
├── remix-strong  Strong remix (v8/v8.1)
├── remix-subtle  Weak remix (v8/v8.1)
└── query         Query task status
```

## Imagine

```bash
aigc-cli midjourney imagine --prompt "a cat wearing a hat"
aigc-cli mj imagine --prompt "a cat" --version 6.1 --speed fast
```

### Image-to-Image

```bash
aigc-cli mj imagine --prompt "a cat" --image-url /path/to/input.png
```

## Blend

Blend 2-4 images:

```bash
aigc-cli mj blend --image-urls /path/to/img1.png,/path/to/img2.png
```

## Describe

Generate prompts from an image:

```bash
aigc-cli mj describe --image-url /path/to/image.png
```

## Upscale

```bash
aigc-cli mj upscale --task-id <task-id> --index 1  # U1
```

## Variation

```bash
aigc-cli mj variation --task-id <task-id> --index 1  # V1
aigc-cli mj high-variation --task-id <task-id> --index 2
aigc-cli mj low-variation --task-id <task-id> --index 3
```

## Zoom / Pan

```bash
aigc-cli mj zoom --task-id <task-id> --zoom 2
aigc-cli mj pan --task-id <task-id> --direction right
```

## Inpaint / Modal

```bash
aigc-cli mj inpaint --task-id <task-id>
aigc-cli mj modal --task-id <task-id> --prompt "new content" --mask /path/to/mask.png
```

## Video

```bash
aigc-cli mj video --task-id <task-id>
```

## Remix

```bash
aigc-cli mj remix-strong --task-id <task-id> --prompt "new prompt"
aigc-cli mj remix-subtle --task-id <task-id> --prompt "new prompt"
```

## Query

```bash
aigc-cli mj query --task-id <task-id>
```

## Configuration

```yaml
defaults:
  midjourney:
    provider: "apimart"       # Midjourney API provider
    speed: "fast"             # fast / relax / turbo
    version: "6.1"            # Midjourney version
    style: "raw"               # Style parameter
    size: "1:1"               # Aspect ratio
```
