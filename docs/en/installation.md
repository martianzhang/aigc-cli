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

See [examples/config.yaml](examples/config.yaml) for a complete reference.

The file may contain API keys, so it is written with owner-only permissions (`0600`) and should stay out of version control.

### Environment Variables

| Variable | Description |
|---|---|
| `OPENAI_API_KEY` | API Key (fallback when config is not set) |
| `OPENAI_BASE_URL` | API base URL (fallback) |
| `AIGC_CLI_MASTER_KEY` | Local encryption master key (`AGE-SECRET-KEY-1…`); overrides the system keyring |
| `AIGC_CLI_NO_KEYRING` | Non-empty: never touch the system keyring (no read, no generate) |
| `HTTP_PROXY` / `HTTPS_PROXY` | HTTP proxy |
| `DOUBAO_API_KEY` | Doubao (Volcengine) search API Key |
| `AIGC_CLI_NO_TITLE` | Non-empty: never write the tab title / progress indicator |
| `AIGC_CLI_TITLE` | `1`: force titles on for an unrecognized terminal |
| `AIGC_CLI_TITLE_BAR` | `1`: force the OSC 9;4 progress bar on unrecognized terminals |

## Proxy

The first run of any `aigc-cli` command generates a local encryption master key
(an age identity) and stores it in the system keyring (macOS Keychain / Windows
Credential Manager / Linux Secret Service). It backs local encryption features
such as the vault. Override it with `AIGC_CLI_MASTER_KEY` (set to the
`AGE-SECRET-KEY-1…` value) for CI, containers and headless hosts; set
`AIGC_CLI_NO_KEYRING` to skip the keyring entirely. Keyring access has a 1s
timeout, and an unavailable keyring only prints a hint — it never fails the
command.

### Secrets in config.yaml are encrypted automatically

Sensitive fields in `config.yaml` (`api_key`, `api_secret`, including under
`providers.*` and `web_search.*`) are **encrypted in place** the next time you
run any `aigc-cli` command — no extra step:

```yaml
api_key: enc:v1:8jjcyC8N4it1sd0ig-xLkmAY47HDqPuXZkWLOr-vuUNqx0Tkd8POFfdq5tl1
```

- Startup scans the file and encrypts any plaintext secret (marked `enc:v1:`),
  then skips them on later runs.
- No plaintext `.bak` is written, and comments/key order are preserved.
- Other fields are untouched, and `config get/list/--print-config` always mask
  secrets.
- On a different machine (or after losing the keyring) decryption fails loudly
  and tells you to set `AIGC_CLI_MASTER_KEY` to the original value; otherwise
  you must re-enter the API key.

### Moving to another machine

Export/import commands for macOS, Linux, and Windows, multi-machine strategies,
rotation, and troubleshooting are covered in
[Master Key & Multi-Machine Migration](guide-master-key.md).

## Proxy

All HTTP requests go through the configured proxy. Configuration priority:

**`--http-proxy` flag > `HTTP_PROXY` / `HTTPS_PROXY` env vars > `http_proxy` in config file**

To bypass the proxy for a single run (for example when a domestic endpoint must be
reached directly), pass `--no-proxy`:

```bash
aigc-cli image --prompt "..." --no-proxy
```

`--no-proxy` ignores the global `http_proxy`, every `providers.*.http_proxy`, and the
`HTTP_PROXY` / `HTTPS_PROXY` environment variables. It is mutually exclusive with an
explicit `--http-proxy <URL>`; combining them is an error.

## Terminal Title & Progress

While a long task runs (async image/video generation, Midjourney/music polling,
large downloads, local ONNX inference) the CLI writes its progress into the
**terminal tab title** and, where the terminal supports it, lights up the
**native progress indicator**, so a background tab still shows how far along the
job is:

```text
aigc-cli · video 45%        # the provider reports a percentage
aigc-cli · image … 1m20s    # the provider only queues — show elapsed time
```

- The title is claimed as soon as the command starts (`aigc-cli · <command> …`), so providers that just queue a job (ModelScope, OpenRouter video) never look hung; providers that report progress replace the ellipsis with `45%`.
- Titles go to `stderr`; `stdout` stays pure data (JSON, saved paths, the MCP stdio protocol).
- The `chat` TUI mirrors its own state: `aigc-cli · chat` (idle), `aigc-cli · chat …` (streaming), `aigc-cli · chat · tool …` (tool call), `aigc-cli · chat · confirm …` (waiting for approval).
- **Only recognized terminals get written to** — an unrecognized one gets nothing, because a lost title always beats garbage on screen:
  - `TERM` starting with `xterm`, `screen`, `tmux`, `rxvt`, `alacritty`, `wezterm`, `kitty`, `ghostty`, `foot`, `konsole`, `vte`, `cygwin`, `msys`, `st-`, `eterm`;
  - or `TERM_PROGRAM` in iTerm2 (`LC_TERMINAL` too) / Terminal.app / WezTerm / Ghostty / VS Code / Hyper / Tabby / mintty;
  - or a Windows console (no `TERM`, and VT mode was switched on successfully).
  - Never: Linux VT (`TERM=linux`), `TERM=dumb`, any unknown `TERM`.
- The indicator uses OSC `9;4`, enabled only for iTerm2, Windows Terminal (`WT_SESSION`), WezTerm, Ghostty, kitty and VS Code; every other terminal gets title-only, and tmux gets title-only too (unless `allow-passthrough` is on).
- Windows: Windows Terminal works for PowerShell, cmd and Git Bash; a legacy conhost first has `ENABLE_VIRTUAL_TERMINAL_PROCESSING` enabled and is skipped entirely when that fails.
- Nothing is written when the stream is not a terminal (pipes, redirects, CI).
- Switches: `AIGC_CLI_NO_TITLE=1` disables everything (highest priority), `AIGC_CLI_TITLE=1` forces titles on for an unrecognized terminal, `AIGC_CLI_TITLE_BAR=1` forces the OSC `9;4` bar.

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
