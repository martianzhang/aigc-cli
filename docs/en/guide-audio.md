# Audio

aigc-cli supports Text-to-Speech (TTS) and Speech-to-Text (STT) via both cloud APIs and local offline models.

## Commands

```
aigc-cli audio
├── tts / speak   Text-to-Speech (cloud API or local sherpa-onnx offline)
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
