# Other Commands

## music

Generate music from a natural-language prompt (APIMart suno/flowmusic async, OpenRouter Lyria sync streaming). See [guide-music.md](guide-music.md).

## models

Query available models and pricing:

```bash
# List all models
aigc-cli models

# List by type
aigc-cli models --type image
aigc-cli models --type video

# View model pricing
aigc-cli models --price
```

> **API key**: calling with no arguments (or with a bare model name, e.g. `aigc-cli models gpt-4o`) hits `/v1/models` and requires an API key. Without `--provider` and a global `api_key` / `OPENAI_API_KEY`, the command fails fast with a clear error listing the configured providers; use `--provider <name>` to pick one, or supply `--api-key`. `--type` and `--price` (marketplace and pricing) are auth-free — no key needed.

## task

Query async task status (APIMart compatible):

```bash
# Use the globally configured provider
aigc-cli task task_01KV4KD9FBH3AZ4DE18A7Y17S3

# Query through a named provider's account and base URL
aigc-cli task --provider apimart task_01KV4KD9FBH3AZ4DE18A7Y17S3
```

`--provider <name>` selects which provider's account and base URL are queried, and the task must belong to that provider. Without it, a missing key fails fast with the list of configured providers that have keys.

## balance

Query account balance across providers:

```bash
# Query every configured provider that has an API key
aigc-cli balance

# Query one specific provider
aigc-cli balance --provider siliconflow

# Query the whole user account
aigc-cli balance user
```

> **API key**: without `--provider`, every configured provider that has an API key is queried (local providers such as Ollama are exempt). An explicitly selected non-local provider without a key fails fast with a clear error; use `--provider <name>` or configure its key.

## config

Read and edit `config.yaml` without opening an editor. Keys are dot paths that
match the YAML structure:

```bash
# Print one value (secrets are masked)
aigc-cli config get defaults.image.model

# Print a whole section (nested secrets are masked too)
aigc-cli config get providers

# Set one value: atomic write, previous content backed up to config.yaml.bak
aigc-cli config set defaults.image.model gpt-image-2

# Print the effective config with secrets masked
aigc-cli config list
```

Notes:

- The file is resolved like every other command: `--config <path>` first, otherwise `~/.config/aigc-cli/config.yaml`. `get`/`set` fail with a clear error when the file is missing — they never create it.
- `set` replaces one leaf only. If a parent section of the dot path does not exist, the command fails (`set defaults.chat.allow_tool_override: section not found`) instead of creating sections.
- The existing YAML type of a key is kept: an int/bool/float stays that type and a value that cannot be parsed as it is rejected; existing strings stay strings. New keys inside an existing section become plain strings unless the value is a plain integer or `true`/`false`.
- Writing is atomic: the previous file is copied to `<path>.bak`, the new content goes to `<path>.tmp.<pid>`, then that file is renamed over the target. Comments, key order and scalar styles survive the round-trip; blank lines and unrelated spacing may be normalized.
- Secrets are never printed in full: `api_key` shows only the last 4 chars (`...abcd`), and credentials inside `base_url` / `http_proxy` become `REDACTED` — the same masking as `--print-config`.
- `api_key` and `base_url` (global or under `providers.*`) require `--force`, because they control where credentials are sent:

```bash
aigc-cli config set api_key sk-xxx
# Error: refusing to set api_key without --force: api_key/base_url hold credentials or endpoint overrides

aigc-cli config set api_key sk-xxx --force
```

- `config list` works without a config file: it prints the code defaults with a `# config file not found` comment.

## dry-run

See what API request would be sent without actually calling:

```bash
aigc-cli image --prompt "a cat" --dry-run
```

Prints the HTTP method, URL, headers, and JSON body.

## completion

Generate shell completion scripts:

```bash
# Bash
aigc-cli completion bash > /etc/bash_completion.d/aigc-cli

# Zsh
aigc-cli completion zsh > /usr/local/share/zsh/site-functions/_aigc-cli

# Fish
aigc-cli completion fish > ~/.config/fish/completions/aigc-cli.fish

# PowerShell
aigc-cli completion powershell > aigc-cli.ps1
```

## Global Flags

| Flag | Description |
|---|---|
| `--dry-run` | Print request params and equivalent curl, no API call |
| `--print-config` | Print effective config with source annotations (secrets masked: API keys show only the last 4 chars, URL credentials/keys become `REDACTED`) |
| `-v` / `--verbose` | Show detailed output: full JSON, token usage, timing, cost |
| `--json` | Pass request as JSON (file, string, or stdin) |
| `--preview` | Open system preview after generation |
| `--save-prompt` | Save prompt as .md file |
| `--http-proxy` | Specify HTTP proxy |
| `--zdr` | Request zero data retention where supported. OpenRouter chat-completions endpoints (`chat`, `music`) honor it via `provider.zdr` + `data_collection: deny`; every other provider/endpoint is a silent no-op |
| `--config` | Path to config file |
| `--api-key` | API key (overrides config/env) |
| `--api-base` | API base URL (overrides config/env) |
| `--model` / `-m` | Model name |
| `--provider` / `-P` | Named provider reference |
| `--output` / `-o` | Output directory |
| `--timeout` | HTTP request timeout in seconds |
