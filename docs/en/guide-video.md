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
| `--image-url` / `-i` | Reference image URL or local file (repeatable); with `--gif` and no `--prompt`, converts a local video to GIF | — |
| `--video-url` | Reference video (APIMart VEO3 Remix) | — |
| `--audio-url` | Reference audio | — |
| `--timeout` | Polling timeout in seconds | `300` |
| `--job-id` | Resume a previous job | — |
| `--gif` | Convert generated videos to GIF after download, or convert a local video via `-i/--image-url` | `false` |
| `--gif-width` | GIF output width in px (height auto, even) | `160` |
| `--ffmpeg-flags` | Extra ffmpeg flags appended after the GIF filter (expert) | — |

## GIF Conversion

`--gif` supports two scenarios: **convert after AI generation**, or **convert an existing local video** (pure local, no API call, no cost).

### After generation

```bash
# Generate and convert to GIF (160px default width)
aigc-cli video --prompt "A man doing push-ups" --gif

# Specify GIF width
aigc-cli video --prompt "A man doing push-ups" --gif --gif-width 320
```

### Convert an existing local video

```bash
# Convert a local video directly (no prompt, no API call)
aigc-cli video --gif -i pushup.mp4              # → pushup_160px.gif

# Specify width / custom output
aigc-cli video --gif -i clip.mp4 --gif-width 320
aigc-cli video --gif -i clip.mov --gif-width 0  # 0 = keep original size
```

Notes:
- Requires system **ffmpeg** on PATH; aigc-cli prints install hints if missing.
- Conversion params are fixed: `fps=6`, palette `max_colors=128`, `dither=none` (no dither pulse on smooth AI-video gradients). Only `--gif-width` is meant to be tuned day-to-day.
- The exact ffmpeg command is printed to stdout, so you can copy and reproduce it.
- Advanced users can append extra args with `--ffmpeg-flags`.
- On the generation path, `--gif` applies to main generation, VEO3 Remix (`--remix`), and `--job-id` resume paths.
- Local conversion triggers when: `--gif` + `-i/--image-url` points to a local video file + **no `--prompt`**. `-i` is the shorthand for `--image-url` (consistent with the `image` command).

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

## Depth Conversion

Depth conversion moved to the dedicated **`aigc-cli depth`** command, which handles
both videos and single images. See [guide-depth.md](guide-depth.md) for the full
guide, parameters, and model table (Depth Anything V2).

```bash
# Convert a video to a grayscale depth video
aigc-cli depth -i input.mp4

# First-time setup
aigc-cli depth init
```

