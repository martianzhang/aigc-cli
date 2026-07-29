# Installation & Configuration

## Installation

### go install

```bash
go install github.com/martianzhang/aigc-cli@latest
```

### Build from Source

```bash
git clone https://github.com/martianzhang/aigc-cli.git
cd aigc-cli
make build
```

### Makefile Commands

```bash
make          # Build
make run ARGS="image --help"   # Run with help
make clean    # Clean build artifacts
make lint     # Static analysis
make test     # Run tests
make cover    # Test coverage
make release  # Cross-compile (all platforms)
```

## Local Generation

aigc-cli can connect to locally running OpenAI-compatible services for image and audio generation without an API Key. aigc-cli automatically detects localhost addresses and skips the Authorization header.

> All local solutions use OpenAI-compatible API formats — aigc-cli needs no code changes.

### Image Generation

```bash
# Ollama (default port 11434)
export OPENAI_BASE_URL="http://localhost:11434/v1"
aigc-cli image --prompt "a cat" --model llava

# Or use --provider local (built-in alias with sensible defaults)
aigc-cli image --provider local --prompt "a cat"
```

### Audio Generation (TTS)

```bash
# openedai-speech (default port 8000)
export OPENAI_BASE_URL="http://localhost:8000/v1"
aigc-cli audio speak --text "Hello world"
```

For local TTS/ASR (sherpa-onnx offline), see [guide-audio.md](guide-audio.md).

## Configuration

### Priority

**CLI flags > JSON input > YAML config > Code defaults**

### Config File

Default path: `~/.config/aigc-cli/config.yaml`

```yaml
api_key: "sk-xxx"
base_url: "https://api.openai.com/v1"

defaults:
  image:
    model: "dall-e-3"
  chat:
    model: "gpt-4o"
```

See [config.example.yaml](config.example.yaml) for a complete reference.

### Environment Variables

| Variable | Description |
|---|---|
| `OPENAI_API_KEY` | API Key (fallback when config is not set) |
| `OPENAI_BASE_URL` | API base URL (fallback) |
| `HTTP_PROXY` / `HTTPS_PROXY` | HTTP proxy |
| `DOUBAO_API_KEY` | Doubao (Volcengine) search API Key |

## Proxy

All HTTP requests go through the configured proxy. Configuration priority:

**`--http-proxy` flag > `HTTP_PROXY` / `HTTPS_PROXY` env vars > `http_proxy` in config file**

## Provider Configuration

Named providers allow you to define reusable API accounts/endpoints:

```yaml
providers:
  my-openai:
    api_key: "sk-xxx"
    base_url: "https://api.openai.com/v1"
  my-openrouter:
    type: openai
    api_key: "sk-or-xxx"
    base_url: "https://openrouter.ai/api/v1"
  my-local:
    type: ollama
    base_url: "http://localhost:11434/v1"

defaults:
  image:
    provider: "my-openai"
  chat:
    provider: "my-openrouter"
```

Provider types: `openai` (default), `ollama` (no API Key needed), `google` (reserved), `anthropic`, `local` (built-in ONNX).
