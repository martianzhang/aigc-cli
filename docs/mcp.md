# MCP Integration

`aigc-cli` implements the [Model Context Protocol](https://modelcontextprotocol.io/) (MCP), allowing AI agents to call image generation, video generation, model queries, and AIGC detection directly — without leaving your chat.

## Supported Clients

### Claude Desktop

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "apimart": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

Set your API key via environment variable before launching:

```bash
export OPENAI_API_KEY="sk-xxx"
export OPENAI_BASE_URL="https://api.apimart.ai"       # or OpenRouter, OpenAI, etc.
```

### Cursor

Add to Cursor's MCP configuration (`~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "apimart": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

### VS Code / Windsurf / Any MCP Host

Same config pattern — every MCP-compatible client uses the same entry:

```json
{
  "mcpServers": {
    "apimart": {
      "command": "aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

Ensure the binary is on your `$PATH`, or use an absolute path:

```json
{
  "mcpServers": {
    "apimart": {
      "command": "/absolute/path/to/aigc-cli",
      "args": ["mcp"]
    }
  }
}
```

## Available Tools

| Tool | Description | API Key Required |
|---|---|---|
| `generate_image` | Image generation (text-to-image, image-to-image, inpainting) | ✅ |
| `generate_video` | Video generation (async submit + poll for result) | ✅ |
| `generate_speech` | Text-to-speech: convert text to spoken audio (TTS) | ✅ |
| `transcribe_audio` | Speech-to-text: transcribe audio file to text (STT) | ✅ |
| `list_models` | List marketplace models, filterable by type | ❌ |
| `get_model_pricing` | Query pricing for a specific model | ❌ |
| `get_balance` | Query token and user balance | ✅ |
| `get_task` | Query async task status and results | ✅ |
| `detect_image` | Detect C2PA / SynthID / TC260 watermarks, AIGC, and EXIF metadata | ❌ |
| `remove_watermark` | Detect and remove a visible AI watermark (Doubao/Jimeng/Baidu/Zhipu), restore original image | ❌ |
| `add_watermark` | Add a visible AI watermark to an image (for testing), known producers use their alpha map | ❌ |

## Configuration

MCP mode reuses the existing config system with three options:

```bash
# Option 1: Config file
# ~/.config/aigc-cli/config.yaml

# Option 2: Environment variables
OPENAI_API_KEY=sk-xxx aigc-cli mcp

# Option 3: CLI flags
aigc-cli mcp --api-key sk-xxx --output ./downloads
```

## Test Available Tools

You can list all MCP tools and their schemas by sending a `tools/list` request via stdio:

```bash
printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}\n{"jsonrpc":"2.0","id":2,"method":"notifications/initialized"}\n{"jsonrpc":"2.0","id":3,"method":"tools/list"}\n' | aigc-cli mcp | jq -r '.result.tools[]?.name // empty'
```

Or pipe to `jq .` for the full formatted JSON of each response message.

## Dynamic Tool Descriptions

On startup, `aigc-cli mcp` reads your `config.yaml` defaults (model, size, resolution, quality, etc.) and injects them into each tool's description. The AI agent sees your current configuration and only overrides parameters when the user explicitly requests it.
