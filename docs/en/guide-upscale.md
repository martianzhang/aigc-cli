# Image Upscaling (`aigc-cli upscale`)

Upscale images **2x / 4x** with local AI models that synthesize detail instead of
simple interpolation. Powered by **Real-ESRGAN / Real-CUGAN / Swin2SR** ONNX
weights on the same ONNX Runtime shared with `background` / `depth` / `ocr` —
**fully offline, no API key**.

Large images are processed in **overlapping tiles** that are feathered back
together, so memory stays bounded and there are no visible tile seams.

## Quick start

```bash
# First use: install ONNX Runtime + the default model
aigc-cli upscale init

# Upscale an image (default general 4x model → <name>_upscaled.png)
aigc-cli upscale -i photo.jpg

# Anime / illustration: Real-CUGAN 2x
aigc-cli upscale -i art.png --model real-cugan-2x

# Explicit output file
aigc-cli upscale -i photo.jpg -o photo_4x.png

# Real-world / compressed photos: Swin2SR RealWorld 4x
aigc-cli upscale -i noisy.jpg --model swin2sr-realworld-x4

# List models + licenses / list what is installed
aigc-cli upscale init --list
aigc-cli upscale init --list-installed

# Print the plan without running
aigc-cli upscale -i photo.jpg --dry-run
```

Output is PNG at `<output_dir>/<name>_upscaled.png` (the output directory is the
global `--output` / `-o`, default current directory).

## Parameters

| Flag | Description | Default |
|---|---|---|
| `--input` / `-i` | Input image file | required |
| `--output` / `-o` | Output file path | `<name>_upscaled.png` |
| `--model` | Model id (see below) | `realesr-general-x4v3` |
| `--scale` | Output factor. **Omitted = the model's native scale**; any other value resamples the result with CatmullRom | model native scale |
| `--dry-run` | Print the plan without running inference | off |
| `--preview` | Open the result with the system default viewer | off |

`upscale init` flags:

| Flag | Description |
|---|---|
| `--model` | Model id(s) to download (repeatable) |
| `--list` | List all models and licenses |
| `--list-installed` | List installed models |
| `--force` | Re-download even if already present |

## Models

| Model | Scale | Size | License | Best for |
|---|---|---|---|---|
| `realesr-general-x4v3` | 4x | 4.9MB | BSD-3-Clause | **Default.** General 4x: balanced speed and quality; the everyday choice |
| `real-esrgan-x4plus` | 4x | 67MB | BSD-3-Clause | High-quality general 4x: best on real-world photos and textures (slower) |
| `real-esrgan-x4plus-anime-6b` | 4x | 18MB | BSD-3-Clause | Anime/illustration 4x (6-block): clean edges and flat colors, ideal for line art |
| `real-esrgan-x4plus-anime-4b32f` | 4x | 5.2MB | BSD-3-Clause | Very small 4-block anime 4x: smaller/faster than 6B, slightly weaker on fine detail |
| `real-esrgan-animevideov3` | 4x | 2.5MB | BSD-3-Clause | Lightest anime/video 4x: fastest; good for animation frames, weaker on fine detail |
| `real-cugan-2x` | 2x | 5.2MB | MIT | Anime 2x: strong line preservation and denoising for illustrations |
| `swin2sr-lightweight-x2` | 2x | 8.1MB | Apache-2.0 | Lightweight general 2x: fast, low resource use |
| `swin2sr-realworld-x4` | 4x | 53MB | Apache-2.0 | Real-world/compressed 4x: noisy or JPEG-compressed photos |
| `swin2sr-classical-x4` | 4x | 55MB | Apache-2.0 | Classical-degradation 4x: high fidelity on clean images |
| `swin2sr-compressed-x4` | 4x | 55MB | Apache-2.0 | Heavily compressed JPEG 4x: recovers detail, suppresses blocking artifacts |

> **Licensing**: model weights keep their upstream license — Real-ESRGAN
> (BSD-3-Clause), Real-CUGAN (MIT), Swin2SR (Apache-2.0). `aigc-cli upscale init
> --list` shows each model's license.

### Choosing a model

| Your input | Recommended model |
|---|---|
| Everyday photos (default, fast) | `realesr-general-x4v3` |
| Photos, maximum quality (slower) | `real-esrgan-x4plus` |
| Anime / illustration / line art | `real-cugan-2x` (2x) or `real-esrgan-x4plus-anime-6b` |
| Anime, smaller/faster than 6B | `real-esrgan-x4plus-anime-4b32f` |
| Animation / video frames (speed) | `real-esrgan-animevideov3` |
| Noisy or JPEG-compressed photos | `swin2sr-realworld-x4` |
| Heavily compressed JPEGs (blocking) | `swin2sr-compressed-x4` |
| Clean images, 4x classical fidelity | `swin2sr-classical-x4` |
| 2x only, lightweight general | `swin2sr-lightweight-x2` |

```bash
aigc-cli upscale init --model real-cugan-2x --model swin2sr-lightweight-x2
```

## How it works

- Input is split into **256×256 tiles** with a **16px overlap**; each tile is run
  through the model and blended back with linear feather weights — bounding
  memory and hiding tile seams.
- Any image size works (no 64-multiple requirement); tiles themselves satisfy
  each model's window alignment.
- Output cap: **16384px** per side, **200MP** total. Exceeding it returns an
  error suggesting a smaller `--scale`.
- Models do not emit an alpha channel, so **transparency is dropped** (output is
  opaque PNG).

## Model location

Models are downloaded to `<models_dir>/upscale/` (default
`~/.config/aigc-cli/models/upscale/`). The ONNX Runtime is **shared** with the
other local commands (`<models_dir>/libonnxruntime.*`) and downloaded once.
`init` respects the configured `http_proxy`.

## Tips

- General photos: `realesr-general-x4v3` (fast) or `real-esrgan-x4plus` (sharper,
  slower). Anime: `real-cugan-2x`.
- A 2x model (Real-CUGAN / Swin2SR-light) with `--scale 4` runs 2x then
  resamples to 4x — for true 4x detail use a native 4x model.
- Speed (Apple Silicon CPU): a 512×512 input with `realesr-general-x4v3` takes a
  few seconds; larger images scale with pixel count.
- Video upscaling is **not built in yet**: extract frames → per-frame
  `upscale` → re-encode (see the `depth` video pipeline). Independent per-frame
  inference can flicker over time.
