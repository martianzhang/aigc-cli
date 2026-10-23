# Audio

aigc-cli supports Text-to-Speech (TTS), Speech-to-Text (STT), and short sound-effect generation (SFX) via both cloud APIs and local offline models.

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
aigc-cli audio speak --text "Hello world" --voice alloy

# OpenRouter (10+ TTS models aggregated)
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
aigc-cli audio speak --text "Hello world" --model "openai/tts-1"
```

## Local TTS (sherpa-onnx)

Download and use local offline TTS:

```bash
# Download models
aigc-cli audio init

# Speak with local kokoro model (53 voices)
aigc-cli audio speak --text "Hello world" --voice zf_xiaoxiao
```

Supported voices include Chinese, English, Japanese, Korean, and French.

## Sound effects (SFX)

`audio sfx` generates a single short effect (whoosh, impact, UI cue, footsteps,
ambience) rather than a full music track. Duration is explicit, seamless
looping is optional, and prompt adherence is tunable.

| Provider | Endpoint | Duration | loop | Notes |
|---|---|---|---|---|
| **ElevenLabs** | `POST /v1/sound-generation` | 0.5–30s (0 = auto) | ✅ | `prompt_influence` |
| **Stability AI** | `POST /v2beta/audio/stable-audio-2/text-to-audio` | 1–190s (default 5s) | ❌ | no `--influence` |

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

# 10-second loopable ambience
aigc-cli audio sfx --input "rain on a car roof, steady ambience" -d 10 --loop

# UI cue with stronger prompt adherence, wav output
aigc-cli audio sfx -i "sci-fi UI confirmation beep" -d 0.8 --influence 0.8 --format wav

# prompt from a file or stdin
aigc-cli audio sfx -i prompt.txt -d 2
echo "cinematic whoosh" | aigc-cli audio sfx -d 1.2

# print the equivalent curl
aigc-cli audio sfx -i "whoosh" -d 1 --dry-run
```

| Flag | Default | Description |
|---|---|---|
| `-i, --input` | stdin | Sound description (file path / text / pipe) |
| `-d, --duration` | `0` | Seconds; 0 = vendor decides (ElevenLabs) or 5s (Stability) |
| `--loop` | `false` | Seamless loop (ElevenLabs only, ≤22s) |
| `--influence` | `0.3` | Prompt adherence 0–1 (ElevenLabs only) |
| `-m, --model` | vendor default | `eleven_text_to_sound_v2` / `stable-audio-2.5` |
| `-f, --format` | `mp3` | `mp3` or `wav` |
| `--play` | `false` | Play after saving |

- Setting only `ELEVENLABS_API_KEY` or `STABILITY_API_KEY` plus
  `--provider elevenlabs|stability` works without a `providers` entry.
- Custom proxies are supported by pointing `base_url` at an ElevenLabs- or
  Stability-compatible endpoint.
- Search first: `aigc-cli search -P sfxmint` finds ready-made CC0 effects; only
  generate when nothing fits.

## Cloud STT

```bash
# OpenAI Whisper
aigc-cli audio stt --file speech.mp3

# OpenRouter
export OPENAI_BASE_URL="https://openrouter.ai/api/v1"
aigc-cli audio stt --file speech.mp3
```

## Local STT (sherpa-onnx)

```bash
# Download models first
aigc-cli audio init

# Transcribe
aigc-cli audio stt --file speech.mp3
```

Local STT uses SenseVoice, which performs best for Chinese.
