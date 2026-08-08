# Depth Conversion (`aigc-cli depth`)

Convert any **image or video** into a **grayscale depth map** — a black-and-white
image/video where pixel brightness encodes relative distance (near = white,
far = black). Textures are removed, leaving only spatial structure. This is the
standard input for depth-guided / control-video image-to-video workflows
(e.g. Wan VACE, Kling Motion Control, Vidu Reference-to-Video): upload the depth
map as a motion/space reference plus a reference photo, and the model generates
new content that keeps the original structure with a new appearance.

Depth is estimated locally with a **Depth Anything V2** ONNX model (default:
`depth-anything-v2-small`, Apache-2.0) — no API key, no network needed.

## Quick start

```bash
# First-time setup: install ONNX Runtime + the default depth model
aigc-cli depth init

# Convert a single image → depth PNG (auto-detected by extension)
aigc-cli depth -i photo.jpg

# Convert a video → depth MP4 (requires ffmpeg)
aigc-cli depth -i video.mp4

# Common options
aigc-cli depth -i photo.jpg --invert          # near = black instead of near = white
aigc-cli depth -i photo.jpg --color           # Spectral_r colored depth (near = warm, far = cool)
aigc-cli depth -i photo.jpg --skeleton        # draw human pose skeleton (COCO17) on the depth image
aigc-cli depth -i photo.jpg --face            # detect faces, draw boxes, eyes and landmarks on the depth image
aigc-cli depth -i video.mp4 --face            # annotate faces on every frame of the depth video
aigc-cli depth -i photo.jpg --preview         # open the result with the system viewer
aigc-cli depth -i video.mp4 --start-time 00:01:00 --end-time 00:01:30
aigc-cli depth -i video.mp4 --encode-args "-crf 28 -preset slow"
aigc-cli depth -i photo.jpg --dry-run         # print what would run, do nothing
```

The input type is auto-detected by file extension:

| Input | Output |
|---|---|
| `.png` / `.jpg` / `.jpeg` / `.webp` / `.bmp` / `.gif` / `.avif` / `.heic` / `.jxl` | `<name>_depth.png` (single-image inference, no ffmpeg needed) |
| `.mp4` / `.mov` / `.mkv` / `.avi` / `.webm` … | `<name>_depth.mp4` (H.264, `yuv420p`, faststart) |

Output is saved next to the input by default, or to `--output <path>`.

## Parameters

| Parameter | Description | Default |
|---|---|---|
| `--input` / `-i` | Input image or video file | required |
| `--output` / `-o` | Output path (else `<name>_depth.png` / `.mp4`) | auto |
| `--model` | Depth model (see table below; aliases: `small`, `base`, `large`) | `depth-anything-v2-small` |
| `--size` | Inference resolution, short side (14-aligned). Images default to `518`; videos default to `280` (fast) — raise to `378`/`518` for higher quality | image `518` / video `280` |
| `--invert` | Invert depth (near = black) | off |
| `--color` | Output a Spectral_r colored depth map (near = warm red/orange, far = cool blue/purple), matching the official Depth-Anything-V2 visualization | off |
| `--start-time` | Video: start time (`SS`, `MM:SS`, `HH:MM:SS`) | video start |
| `--end-time` | Video: end time; alone = convert the first N seconds | video end |
| `--keep-audio` | Video: keep the source audio track | off |
| `--encode-args` | Video: extra ffmpeg encode args, **appended after the defaults** — same-named options override them (last wins). Examples: `"-crf 28"` (smaller file), `"-preset slow"` (better compression) | CRF 23, preset medium |
| `--no-smooth` | Video: disable temporal smoothing (reduces flicker) | off (smoothing on) |
| `--parallel` / `-p` | Video: number of parallel inference workers. Auto-tuned by CPU cores (default min(perf cores, 4)); raise for large clips, lower to save memory | auto |
| `--preview` | Open the depth result with the system default viewer | off |
| `--skeleton` | Detect human poses and draw COCO17 skeleton (19 bones, 17 joints) on the depth output (image or video, per-frame). Requires `aigc-cli depth init --skeleton` | off |
| `--face` | Detect faces (pure-Go pigo engine) and draw boxes, eyes and 15 facial landmarks on the depth output (image or video, per-frame). Requires `aigc-cli depth init --face` | off |
| `--dry-run` | Print what would run without doing it | off |

## Models

| Model | Params | Size | License | Notes |
|---|---|---|---|---|
| `depth-anything-v2-small` | 24.8M | 99MB | Apache-2.0 | **Default.** Fastest, clean output — smooths fine texture, highlights main structure |
| `depth-anything-v2-base` | 97.5M | 370MB | CC-BY-NC-4.0 | Higher quality, non-commercial |
| `depth-anything-v2-large` | 335M | 1.25GB | CC-BY-NC-4.0 | Highest quality, non-commercial |

> **Licenses matter for commercial use**: `depth-anything-v2-small` is
> Apache-2.0 (commercial friendly). `-base` and `-large` are CC-BY-NC-4.0
> (non-commercial).

Download a specific model with `aigc-cli depth init --model <id>`, or all
variants with `aigc-cli depth init --all`.

## Requirements

- **Images**: nothing extra — ONNX Runtime + model via `aigc-cli depth init`.
- **Videos**: additionally **ffmpeg** on your PATH (aigc-cli does not bundle it).
  See [guide-ffmpeg.md](guide-ffmpeg.md) for install hints per platform.

## Tips

- Image inference is fast: roughly 1–3 seconds on CPU depending on the model.
- Video inference runs at roughly 3 frames/second on CPU (V2-Small) — a
  10-second 30fps clip takes about 2 minutes. Preview a short segment with
  `--end-time` first, tune `--invert` / `--no-smooth`, then convert the full range.
- The depth video has no audio by default; add `--keep-audio` if you want the
  original soundtrack.
- `--dry-run` prints the two ffmpeg commands (frame extraction + encoding) so
  you can inspect or tweak them manually.
- In chat or via MCP, the equivalent tool is `convert_depth` (same auto-routing).
