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
| `--image-url` / `-i` | Reference image URL or local file (repeatable); with `--gif`/`--mp4` and no `--prompt`, converts a local file | — |
| `--video-url` | Reference video (APIMart VEO3 Remix) | — |
| `--audio-url` | Reference audio | — |
| `--timeout` | Polling timeout in seconds | `300` |
| `--job-id` | Resume a previous job | — |
| `--gif` | Convert generated videos to GIF after download, or convert a local video via `-i/--image-url` | `false` |
| `--gif-width` | GIF output width in px (height auto, even) | `160` |
| `--mp4` | Convert a local media file (GIF/WebP/MOV/…) to MP4 via `-i/--image-url` (local only; requires ffmpeg; mutually exclusive with `--gif`) | `false` |
| `--crop-margin` | Crop N px from each side of the video (crops exactly what you specify). Without a prompt: crops the local video (`-i file`), original kept. With a prompt: crops AI-generated videos, originals kept. CSS margin shorthand: `40` (all) / `40,0` (top/bottom, left/right) / `40,30,20,10` (top,right,bottom,left) | `—` |
| `--ffmpeg-flags` | Extra ffmpeg flags appended after the GIF filter (expert) | — |

> **Note:** Agnes video models (`agnes-video-2.5-flash`, etc.) require `--resolution` in `720P`/`960P`/`2K` format (uppercase P), not `480p`/`720p`. The CLI maps the default `480p` to `720P` automatically. See the [Agnes Video docs](https://agnes-ai.com/zh-Hans/docs/agnes-video-v20).

### Agnes Image-to-Video

Agnes has no image upload endpoint; local images are automatically converted to base64 data URIs and embedded directly. Three modes are supported:

| Mode | Description | CLI Usage |
|---|---|---|
| `text` | Pure text-to-video (default) | `--prompt "..."` |
| `reference` | Image/audio reference | `-i image1.jpg -i image2.jpg` (up to 5 images) |
| `keyframe` | First/last frame control | `--first-frame img1.jpg --last-frame img2.jpg` |

```bash
# Image-to-video: local images auto-converted to data URI
aigc-cli video -i photo.jpg --prompt "animate the person in the photo" --gif

# Keyframe
aigc-cli video --first-frame start.jpg --last-frame end.jpg --prompt "transition from first to last frame"
```

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

# Crop the surrounding frame: crop 40px from each side
aigc-cli video --gif -i org.mp4 --crop-margin 40

# Crop only top/bottom bars (CSS margin shorthand: top/bottom, left/right)
aigc-cli video --gif -i org.mp4 --crop-margin 40,0

# Crop only the bottom edge (top, right, bottom, left)
aigc-cli video --gif -i org.mp4 --crop-margin 0,0,40,0
```

Notes:
- Requires system **ffmpeg** on PATH; aigc-cli prints install hints if missing.
- Conversion params are fixed: `fps=6`, palette `max_colors=128`, `dither=none` (no dither pulse on smooth AI-video gradients). Only `--gif-width` is meant to be tuned day-to-day.
- The exact ffmpeg command is printed to stdout, so you can copy and reproduce it.
- Advanced users can append extra args with `--ffmpeg-flags`.
- On the generation path, `--gif` applies to main generation, VEO3 Remix (`--remix`), and `--job-id` resume paths.
- Local conversion triggers when: `--gif` + `-i/--image-url` points to a local video file + **no `--prompt`**. `-i` is the shorthand for `--image-url` (consistent with the `image` command).

## Media to MP4

`--mp4` converts a local media file (GIF / WebP / APNG / MOV / MKV / WebM / AVI — any ffmpeg-decodable format) to MP4 (H.264 + yuv420p, best compatibility). Pure local: no API call, no cost.

```bash
# GIF to MP4: anim.gif → anim.mp4
aigc-cli video --mp4 -i anim.gif

# Other formats (MOV / WebM / …); audio is preserved automatically
aigc-cli video --mp4 -i clip.mov

# Crop the edges first, then convert (output keeps the source resolution)
aigc-cli video --mp4 -i anim.gif --crop-margin 10
```

Notes:
- Requires system **ffmpeg** on PATH; install hints are printed if missing.
- Output is `<stem>.mp4` next to the input; the **original file is preserved**.
- Output keeps the source resolution; dimensions are auto-rounded to even (H.264 requires even sizes); transparency in GIF/WebP is flattened to black.
- Audio is preserved when the source has it; no audio track is produced when it does not.
- If the output would be the same file as the input (e.g. converting an `.mp4` directly), it errors out instead of overwriting.
- The exact ffmpeg command is printed to stdout; `--ffmpeg-flags` appends extra args.
- Mutually exclusive with `--gif`; local files only (`-i file` + no `--prompt`).

## Edge Crop

`--crop-margin` works **standalone** (no `--gif` needed) to re-encode a video with the border edges cropped off. The original video is always kept; a new `<stem>_crop.mp4` file is written next to it. Only the specified sides are cropped — e.g. `40,0` removes 40px from top/bottom and leaves the left/right untouched.

```bash
# Crop a local video (no prompt, no API call): org.mp4 → org_crop.mp4
aigc-cli video --crop-margin 40 -i org.mp4
aigc-cli video --crop-margin 40,0 -i org.mp4        # crop only top/bottom bars

# Crop AI-generated videos after download (originals kept)
aigc-cli video --prompt "A man doing push-ups" --crop-margin 40
aigc-cli video --prompt "..." --crop-margin 0,0,40,0  # crop only the bottom edge
```

Notes:
- `--crop-margin` accepts CSS margin shorthand (comma-separated): 1 value = all sides, 2 values = top/bottom, left/right, 4 values = top, right, bottom, left. It crops exactly the given px from the selected sides — no extra cropping on other sides. `ffprobe` (ships with ffmpeg) is used to validate the margin against the source size.
- Standalone crop is pure local re-encoding (H.264) via ffmpeg — no API call, no cost.
- `--crop-margin` also applies to VEO3 Remix (`--remix`) and `--job-id` resume paths.

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

## Pollinations Video

When the base URL points at `pollinations.ai`, aigc-cli uses its synchronous media
endpoint `GET /video/{prompt}`, which returns raw MP4 bytes:

```bash
# Text-to-video (community model)
aigc-cli video --provider pollinations \
  --model "community/NamanSoni78/Seedance-2.5" \
  --prompt "a cat walking in a garden" --duration 4

# Image-to-video (start frame)
aigc-cli video --provider pollinations \
  --model "community/NamanSoni78/Seedance-2.5" \
  --prompt "the cat walks forward" --image-url ./cat.jpg
```

Notes:
- The endpoint lives at the API root, `https://gen.pollinations.ai/video/{prompt}` — **not** under `/v1`. `model`, `duration`, and `seed` are passed as query parameters; a start-frame image is passed as the `image` parameter (`--image-url` and `--first-frame` both map to it).
- **Cost depends on the model's price, not on whether it is a community model.** A model is free only when its advertised `pricing` is zero — inspect it with `GET /video/models` (or `aigc-cli models --provider pollinations`). Official models (`google/veo-3.1-fast`, `bytedance/seedance-*`, `alibaba/wan-*`, …) are `paid_only` and require **paid Pollen**, otherwise the request fails with `402 Insufficient balance`. Community models are **not** automatically free: `community/NamanSoni78/Seedance-2.5`, for example, is priced at `completionVideoSeconds: 0.25` and consumes Pollen; only zero-priced community models such as `community/ZapGaming/failure-reel-v1` cost nothing.
- Pollinations' video endpoint **does not accept `--resolution`** (most models return 400), so the CLI does not forward it; `--size` is likewise unused.
- Generation is synchronous and typically takes 1–3 minutes. Pollinations keeps generating after the client disconnects, so on timeout simply re-issue the identical request (it hits the cache and is not billed twice).

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

