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

### Are local features free?

Yes — AIGC detection, OCR, background removal, TTS/ASR with local models are all free.
