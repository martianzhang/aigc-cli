# ffmpeg Common Commands Cheat Sheet

`aigc-cli video` / `audio` commands handle **generation and download**. Post-processing (frame extraction, thumbnails, GIF conversion, clipping, transcoding, audio extraction) can be done directly with the system-installed **ffmpeg** — no extra integration needed.

> Why not built-in? ffmpeg is the de-facto standard: feature-complete, mature ecosystem, and calling the CLI is the simplest reliable approach. aigc-cli stays a single binary without bundling ffmpeg.

## Check Installation

```bash
ffmpeg -version   # Not installed: brew install ffmpeg / apt install ffmpeg / choco install ffmpeg
```

## Video Post-Processing (with the `video` command)

Videos generated via `aigc-cli video --prompt "..."` are saved as mp4 in the current directory by default. The commands below use it as input.

### Extract Frames (preview content)

```bash
# Extract one frame at 3s
ffmpeg -i video_xxx.mp4 -ss 3 -frames:v 1 frame_3s.png

# Extract one frame per second (contact sheet / frame-by-frame check)
ffmpeg -i video_xxx.mp4 -vf fps=1 frame_%03d.png
```

### First-Frame Thumbnail

```bash
ffmpeg -i video_xxx.mp4 -frames:v 1 -vf scale=320:-1 thumb.jpg
```

### Convert to GIF (social sharing)

> Use the built-in **`aigc-cli video --gif`** for video → GIF: auto-convert after generation (`video -p "..." --gif`) or convert an existing local video (`video --gif -i file.mp4`), see [guide-video.md](guide-video.md#gif-conversion). The raw ffmpeg commands below are kept for reference.

```bash
# Basic conversion (acceptable quality)
ffmpeg -i video_xxx.mp4 -vf fps=10,scale=480:-1 out.gif

# High-quality palette mode (recommended: smaller, no banding)
ffmpeg -i video_xxx.mp4 -vf "fps=12,scale=480:-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" out.gif
```

### Clip a Segment

```bash
# Clip 2s–5s (-t is duration, not end time)
ffmpeg -i video_xxx.mp4 -ss 2 -t 3 -c copy clip.mp4

# Accurate clipping (re-encode; -ss before -i is frame-accurate)
ffmpeg -ss 2 -i video_xxx.mp4 -t 3 -c:v libx264 -crf 18 clip.mp4
```

### Concatenate Clips

```bash
# 1. Write a file list
echo "file 'clip1.mp4'"  > list.txt
echo "file 'clip2.mp4'" >> list.txt

# 2. Concatenate (lossless with -c copy when encodings match)
ffmpeg -f concat -safe 0 -i list.txt -c copy merged.mp4
```

### Extract Audio

```bash
ffmpeg -i video_xxx.mp4 -vn -c:a libmp3lame -q:a 2 audio.mp3
ffmpeg -i video_xxx.mp4 -vn -c:a pcm_s16le audio.wav
```

### Transcode / Reduce Size

```bash
# H.264 + AAC (most compatible upload format)
ffmpeg -i video_xxx.mp4 -c:v libx264 -crf 23 -c:a aac out.mp4

# Downscale to 720p to reduce size
ffmpeg -i video_xxx.mp4 -vf scale=-2:720 -c:v libx264 -crf 26 -c:a aac out_720p.mp4

# Strip audio track
ffmpeg -i video_xxx.mp4 -an out_silent.mp4
```

> `-crf` controls quality: 18 ≈ visually lossless, 23 default, 28 heavily compressed. Lower value = larger file, better quality.

### Speed / Reverse

```bash
ffmpeg -i video_xxx.mp4 -filter:v "setpts=0.5*PTS" -filter:a "atempo=2.0" out_2x.mp4
ffmpeg -i video_xxx.mp4 -vf reverse -af areverse out_reversed.mp4
```

## Audio Processing (with the `audio` command)

### Convert to 16k Mono WAV before Local ASR

Local sherpa-onnx recognition (`aigc-cli audio asr --local`) requires 16 kHz mono WAV:

```bash
ffmpeg -i input.mp3 -ar 16000 -ac 1 out_16k.wav
```

### Other Common Operations

```bash
# Clip the first 30 seconds
ffmpeg -i input.mp3 -t 30 clip.mp3

# Boost volume 2x (limiter prevents clipping)
ffmpeg -i input.mp3 -af "volume=2.0,alimiter=limit=0.95" louder.mp3

# Concatenate two audio files
ffmpeg -i a.mp3 -i b.mp3 -filter_complex "[0:a][1:a]concat=n=2:v=0:a=1" merged.mp3

# Convert TTS output to mp3 for sharing
ffmpeg -i tts.wav -c:a libmp3lame -q:a 2 tts.mp3
```

## Common Options Cheat Sheet

| Option | Purpose | Example |
|---|---|---|
| `-i <file>` | Specify input | `-i video.mp4` |
| `-ss <t>` | Start at t seconds | `-ss 3` (`1:30` format supported) |
| `-t <dur>` | Process duration | `-t 3` |
| `-frames:v N` | Output only N frames (frame extraction) | `-frames:v 1` |
| `-vf` | Video filter chain (comma-separated) | `-vf scale=480:-1,fps=10` |
| `-af` | Audio filter chain | `-af volume=2.0` |
| `-c:v` | Video codec | `-c:v libx264` / `-c copy` to copy without re-encoding |
| `-c:a` | Audio codec | `-c:a aac` / `-c:a pcm_s16le` |
| `-crf` | Quality 0-51 (lower is better) | `-crf 23` |
| `-an` / `-vn` | Drop audio / video track | `-an` |
| `-y` | Overwrite output file | `-y` |

## Tips

- **`-c copy` is lossless and instant**, but requires matching input/output encodings; drop it only when re-encoding is needed (accurate clipping, transcoding).
- **Filter order matters**: put `fps` after `scale` — lowering frame rate first saves CPU.
- More usage: `ffmpeg -h`, official docs <https://ffmpeg.org/documentation.html>.
