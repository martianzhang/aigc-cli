# Other Commands

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

## task

Query async task status (APIMart compatible):

```bash
aigc-cli task <task-id>
```

## balance

Query account balance (APIMart compatible):

```bash
aigc-cli balance
```

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
| `--print-config` | Print effective config with source annotations |
| `-v` / `--verbose` | Show detailed output: full JSON, token usage, timing, cost |
| `--json` | Pass request as JSON (file, string, or stdin) |
| `--preview` | Open system preview after generation |
| `--save-prompt` | Save prompt as .md file |
| `--http-proxy` | Specify HTTP proxy |
| `--config` | Path to config file |
| `--api-key` | API key (overrides config/env) |
| `--api-base` | API base URL (overrides config/env) |
| `--model` / `-m` | Model name |
| `--provider` | Named provider reference |
| `--output` | Output directory |
| `--timeout` | HTTP request timeout in seconds |
