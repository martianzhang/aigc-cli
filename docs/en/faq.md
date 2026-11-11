# FAQ

## General

### What APIs does aigc-cli support?

OpenAI, OpenRouter, APIMart, and any OpenAI-compatible relay. Also supports Ollama for local models.

### Do I need an API Key?

For OpenAI/OpenRouter/APIMart — yes. For local models (Ollama, ONNX) — no.

### How is this different from using curl?

aigc-cli handles provider auto-detection, async task polling, Midjourney API translation, MCP integration, and local AIGC detection.

## Installation

### How to install?

```bash
go install github.com/martianzhang/aigc-cli@latest
```

Or download from [Releases](https://github.com/martianzhang/aigc-cli/releases).

### Does it work on Windows?

Yes, the Go binary is cross-platform.

## MCP

### How do I set up MCP?

Add to Claude Desktop / Cursor config:

```json
{
  "mcpServers": {
    "aigc-cli": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

### Can I restrict which tools the AI can use?

Yes, use `tools_enable` / `tools_disable` in config.yaml.

## Cost

### Does kb search cost money?

Depends on the provider:
- duckduckgo: free
- doubao (Volcengine): 0.020 CNY/request (500 free/month)
- firecrawl: paid (has free tier)
- deepseek: $0.75/1000 successful requests (≈¥5/1000)
- exa: $7/1000 auto searches + $1/1000 contents pages (free tier: $10 credit/month)

### Are local features free?

Yes — AIGC detection, OCR, background removal, TTS/ASR with local models are all free.

## Preview

### Why does `--preview` print a wall of base64?

That blob is not the API response — it is the image itself. In iTerm2 / Kitty / Sixel terminals `--preview` (and `preview`) renders inline by writing an OSC 1337 escape to stdout, whose payload is the base64-encoded image. Older builds wrote it unconditionally, so when stdout was a pipe, a log, or captured by an IDE/TUI the escape was printed as literal text.

Now the escape is only written when stdout is a real terminal, and the payload is a downscaled, re-encoded preview copy (JPEG, or PNG when the image has transparency) capped at 512 KB. Anything larger is opened with the system viewer instead.

To always skip inline rendering:

```bash
AIGC_CLI_NO_INLINE_PREVIEW=1 aigc-cli image ... --preview
```

See [Preview](guide-preview.md) for details.
