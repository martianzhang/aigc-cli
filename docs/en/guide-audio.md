# Audio

aigc-cli supports Text-to-Speech (TTS), Speech-to-Text (STT), and short sound-effect generation (SFX) via both cloud APIs and local offline models.

> `audio speak` and `audio sfx` accept their text input as `--input` or its alias `--prompt` / `-p`. (`audio transcribe --input` is an audio file path, so it has no `--prompt` alias.)

## Commands

```
aigc-cli audio
├── tts / speak   Text-to-Speech (cloud API or local sherpa-onnx offline)
├── sfx           Short sound effects (ElevenLabs / Stability AI)
├── asr / stt     Speech-to-Text (cloud API or local sherpa-onnx offline)
└── init          Download local models (kokoro, sense-voice, etc.)
```

## Cloud TTS

```bash
# OpenAI TTS
aigc-cli audio speak -i "Hello world" --voice alloy
aigc-cli audio speak --prompt "Hello world" --voice alloy   # alias for --input

# OpenRouter (10+ TTS models aggregated)
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
aigc-cli audio speak -i "Hello world" --model "openai/tts-1"
```

## Local TTS (sherpa-onnx)

Download and use local offline TTS:

```bash
# Download models
aigc-cli audio init

# Speak with local kokoro model (53 voices)
aigc-cli audio speak -P local -i "Hello world" --voice zf_xiaoxiao

# Print the local plan only (no model load, no file written)
aigc-cli audio speak -P local -i "Hello world" --dry-run
```

Supported voices include Chinese, English, Japanese, Korean, and French.

## Sound effects (SFX)

`audio sfx` generates a single short effect (whoosh, impact, UI cue, footsteps,
ambience) rather than a full music track. Duration is explicit, seamless
looping is optional, and prompt adherence is tunable.

| Provider | Endpoint | Duration | Vendor fields (`--inject-json`) |
|---|---|---|---|
| **ElevenLabs** | `POST /v1/sound-generation` | 0.5–30s (0 = auto) | `loop`, `prompt_influence`, … |
| **Stability AI** | `POST /v2beta/audio/stable-audio-2/text-to-audio` | 1–190s (default 5s) | `cfg_scale`, `seed`, … |

> Vendor-specific knobs are not normalized into CLI flags; pass them through
> `--inject-json` so each new provider does not add another flag.

```yaml
# ~/.config/aigc-cli/config.yaml
providers:
  elevenlabs:
    base_url: https://api.elevenlabs.io
    api_key: "xi-xxx"
defaults:
  audio:
    sfx:
      provider: elevenlabs
      # model: eleven_text_to_sound_v2
      # format: mp3
```

```bash
# 1.5-second door slam
aigc-cli audio sfx --input "heavy metal door slam with echo" --duration 1.5
aigc-cli audio sfx --prompt "heavy metal door slam with echo" --duration 1.5

# 10-second loopable ambience (vendor-native loop field)
aigc-cli audio sfx -i "rain on a car roof, steady ambience" -d 10 \
  --inject-json '{"loop":true,"prompt_influence":0.5}'

# UI cue with stronger prompt adherence, wav output
aigc-cli audio sfx -i "sci-fi UI confirmation beep" -d 0.8 \
  --inject-json '{"prompt_influence":0.8}' --format wav

# prompt from a file or stdin
aigc-cli audio sfx -i prompt.txt -d 2
echo "cinematic whoosh" | aigc-cli audio sfx -d 1.2

# print the equivalent curl
aigc-cli audio sfx -i "whoosh" -d 1 --dry-run
```

| Flag | Default | Description |
|---|---|---|
| `-i, --input` | stdin | Sound description (file path / text / pipe) |
| `-p, --prompt` | — | Alias for `--input` |
| `-d, --duration` | `0` | Seconds; 0 = vendor decides (ElevenLabs) or 5s (Stability) |
| `--inject-json` | empty | vendor-native fields (JSON object/file); e.g. `loop`, `prompt_influence` for ElevenLabs, `cfg_scale`/`seed` for Stability |
| `-m, --model` | vendor default | `eleven_text_to_sound_v2` / `stable-audio-2.5` |
| `-f, --format` | `mp3` | `mp3` or `wav` |
| `--play` | `false` | Play after saving |

- Setting only `ELEVENLABS_API_KEY` or `STABILITY_API_KEY` plus
  `--provider elevenlabs|stability` works without a `providers` entry.
- Custom proxies are supported by pointing `base_url` at an ElevenLabs- or
  Stability-compatible endpoint.
- Search first: `aigc-cli search -P sfxmint` finds ready-made CC0 effects; only
  generate when nothing fits.

### Local SFX (offline ONNX)

Local mode runs Stable Audio Open Small (341M, ONNX) with pure Go inference. No
API key and no Python runtime are required.

```bash
# 1. download the model (~507MB; reuses the installed ONNX Runtime)
aigc-cli audio init --model stable-audio-open-small

# 2. generate locally (5s default, peak-normalized to -1 dBFS)
aigc-cli audio sfx -P local -m stable-audio-open-small \
  -i "heavy metal door slam" -d 2

# 3. fixed seed / raw levels (local engine knobs also use --inject-json)
aigc-cli audio sfx -P local -i "cinematic whoosh" -d 1.2 \
  --inject-json '{"seed":42}'
aigc-cli audio sfx -P local -i "rain on a roof" -d 8 \
  --inject-json '{"normalize":false}'   # default is -1 dBFS peak normalize
```

To make local the default (no `-P local` needed):

```yaml
defaults:
  audio:
    sfx:
      provider: local              # or pass -P local per invocation
      # model: stable-audio-open-small
```

Local inference is the built-in `local` provider, so `-P` works exactly like
`image` / `music`; `-m` selects the local model ID (default
`stable-audio-open-small`).

| Flag | Description |
|---|---|
| `-P, --provider` | use `local` for local inference |
| `-m, --model` | local model ID, default `stable-audio-open-small` (absolute paths accepted) |
| `-d, --duration` | 0.1–11.9s; defaults to 5s when 0 |
| `--inject-json` | Local engine knobs: `steps` (1–50), `sigma_max` (0.01–1), `seed`, `normalize`; online passes vendor fields |

- Output is always **44.1kHz stereo WAV**; `--format mp3` is online-only.
  `--inject-json` works for both, with different keys.
- The model caps clips at 11.9s and rejects longer requests.
- Reference speed: about 9s for a 1s clip on Apple Silicon CPU (8 DiT steps +
  VAE decode).
- `--dry-run` prints the resolved plan (model, duration, seed/steps, planned
  output path) without loading the model or writing a file.
- License: Stability AI Community License (commercial use under $1M revenue,
  attribution retained).

## Cloud STT

```bash
# OpenAI Whisper
aigc-cli audio transcribe -i speech.mp3

# OpenRouter
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
aigc-cli audio transcribe -i speech.mp3
```

## Local STT (sherpa-onnx)

```bash
# Download models first
aigc-cli audio init

# Transcribe
aigc-cli audio transcribe -i speech.mp3

# Print the local plan only (no model load, no file written)
aigc-cli audio transcribe -i speech.mp3 --dry-run
```

Local STT uses SenseVoice, which performs best for Chinese.
