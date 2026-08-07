# Video Generation

Use `aigc-cli video` (alias `vid`) to generate videos.

## Basic Usage

```bash
# Simple prompt
aigc-cli video --prompt "a dog running on the beach"

# With model
aigc-cli video --model "google/veo-3.1" --prompt "a dog running"
```

## Parameters

| Parameter | Description | Default |
|---|---|---|
| `--prompt` / `-p` | Video description prompt | required |
| `--model` / `-m` | Model name | `grok-imagine-1.5-video-apimart` (APIMart) |
| `--size` | Aspect ratio (`16:9`, `9:16`, `1:1`) | `16:9` |
| `--duration` | Duration in seconds | model default |
| `--image-url` | First frame reference image | — |
| `--video-url` | Reference video (APIMart VEO3 Remix) | — |
| `--audio-url` | Reference audio | — |
| `--timeout` | Polling timeout in seconds | `300` |
| `--job-id` | Resume a previous job | — |

## OpenRouter Video

aigc-cli submits to OpenRouter's dedicated video API, polls for completion, and downloads the result:

```bash
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
export OPENAI_API_KEY="sk-or-xxx"

aigc-cli video --model "google/veo-3.1" --prompt "a dog running"
```

### Resume with --job-id

If polling times out, you can resume with the job ID:

```bash
aigc-cli video --job-id "abc123"
```

The job ID is printed when the job is submitted:

```
Submitted job: abc123
```

## APIMart Video

APIMart uses async task submission:

```bash
aigc-cli video --prompt "a dog running"
```

### First Frame / Last Frame

```bash
aigc-cli video --prompt "a dog" --image-url /path/to/first_frame.png
```

### Reference Video (VEO3 Remix)

Extend a reference video:

```bash
aigc-cli video --prompt "keep going" --video-url /path/to/reference.mp4
```

## Yunwu AI Video

```bash
export OPENAI_BASE_URL="https://yunwu-api.example.com"
aigc-cli video --prompt "a dog running"
```

## JSON Input

```bash
aigc-cli video --json '{"prompt": "a dog running", "model": "google/veo-3.1"}'
```

## Depth Conversion (`--convert-to-depth`)

Convert any video into a **grayscale depth video** — a black-and-white video where
pixel brightness encodes relative distance (near = white, far = black). Textures
are removed, leaving only motion and spatial structure. This is the standard
input format for depth-guided / control-video image-to-video workflows
(e.g. Wan VACE, Kling Motion Control, Vidu Reference-to-Video): upload the depth
video as a motion/space reference plus a reference photo, and the model generates
a new video that keeps the original motion with the new appearance.

Depth is estimated locally with a **Depth Anything V2** ONNX model (default:
`depth-anything-v2-small`, Apache-2.0) — no API key, no network needed.

```bash
# First-time setup: install ONNX Runtime + the depth model (default = small)
aigc-cli video init

# Convert the whole video
aigc-cli video --convert-to-depth -i input.mp4

# Convert only a time range (--end-time alone = first N seconds)
aigc-cli video --convert-to-depth -i input.mp4 --start-time 00:01:00 --end-time 00:01:30

# Different model / options
aigc-cli video --convert-to-depth -i input.mp4 --depth-model depth-anything-v2-large --invert

# See the exact ffmpeg commands without running anything
aigc-cli video --convert-to-depth -i input.mp4 --dry-run
```

The output is saved next to the input as `<input_stem>_depth.mp4`
(H.264, `yuv420p`, faststart) in the output directory.

### Depth Parameters

| Parameter | Description | Default |
|---|---|---|
| `--convert-to-depth` | Enable depth conversion mode | off |
| `--input` / `-i` | Input video file | required |
| `--start-time` | Start time (`SS`, `MM:SS`, `HH:MM:SS`) | video start |
| `--end-time` | End time; alone = convert the first N seconds | video end |
| `--depth-model` | Model: `depth-anything-v2-small` / `-base` / `-large` (aliases: `small`, `base`, `large`) | `depth-anything-v2-small` |
| `--depth-size` | Inference resolution, short side (14-aligned). Default 280 (fast, depth structure readable); raise to `378` or `518` for higher quality | `280` |
| `--invert` | Invert depth (near = black) | off |
| `--no-smooth` | Disable temporal smoothing (reduces flicker) | off (smoothing on) |
| `--keep-audio` | Keep the source audio track | off |

> **Models & licenses**: `depth-anything-v2-small` is Apache-2.0 (commercial
> friendly, the default). `depth-anything-v2-base` and `-large` are
> CC-BY-NC-4.0 (non-commercial). Download them explicitly with
> `aigc-cli video init --model depth-anything-v2-base`.

### Requirements

- **ffmpeg** on your PATH (aigc-cli does not bundle it). See
  [guide-ffmpeg.md](guide-ffmpeg.md) for install hints per platform.
- **ONNX Runtime** and the depth model: `aigc-cli video init`.

### Tips

- Depth inference runs at roughly 3 frames/second on CPU — a 10-second 30fps
  clip takes about 2 minutes to convert. Use `--end-time` to preview a short
  segment first, tune `--invert` / `--no-smooth`, then convert the full range.
- The output has no audio by default (depth videos are motion references);
  add `--keep-audio` if you want the original soundtrack.
- `--dry-run` prints the two ffmpeg commands (frame extraction + encoding) so
  you can inspect or tweak them manually.

