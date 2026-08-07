# MCP Integration

aigc-cli provides a built-in MCP (Model Context Protocol) server, allowing AI agents (Claude Desktop, Cursor, Windsurf, VS Code) to generate images, create videos, search ideas, and more directly in conversation.

## Quick Start

Add to your MCP config file:

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

For Claude Desktop: `~/Library/Application Support/Claude/claude_desktop_config.json`
For Cursor/Windsurf: `.cursor/mcp.json` or Settings → MCP

## Available Tools

| Tool | Description | Cost |
|---|---|---|
| `generate_image` | Generate images | Paid (API) |
| `generate_video` | Generate videos | Paid (API) |
| `generate_speech` | Text-to-speech | Paid/Free |
| `transcribe_audio` | Speech-to-text | Paid/Free |
| `midjourney_imagine` | Midjourney imagine | Paid (API) |
| `midjourney_upscale` | Midjourney upscale | Paid (API) |
| `midjourney_variation` | Midjourney variation | Paid (API) |
| `search_ideas` | Prompt ideas search | Free (local) |
| `detect_image` | AIGC detection | Free (local) |
| `remove_watermark` | Watermark removal (AI inpainting) | Free (local) |
| `add_watermark` | Watermark addition | Free (local) |
| `crop_watermark` | Crop to remove watermarks (no learning required) | Free (local) |
| `remove_background` | Background removal | Free (local) |
| `convert_video_depth` | Video → grayscale depth video (Depth Anything V2) | Free (local) |
| `recognize_text` | OCR text recognition | Free (local) |
| `caption_image` | EXIF description read/write | Free (local) |
| `kb_find` | Knowledge base search | Free (local) |
| `kb_search` | Web search + KB save | Free/Paid |
| `kb_list` | List KB documents | Free (local) |
| `kb_show` | Show KB document details | Free (local) |
| `get_task` | Query async task status | Free |
| `list_models` | List available models | Free |
| `get_balance` | Query account balance | Free |
| `get_model_pricing` | Query model pricing | Free |

## Tool Filtering

Control which tools are available via config:

```yaml
# Allowlist — only free/offline tools
tools_enable:
  - "search_ideas"
  - "detect_image"
  - "remove_watermark"
  - "crop_watermark"
  - "remove_background"
  - "convert_video_depth"
  - "recognize_text"

# Blocklist — disable paid tools
tools_disable:
  - "generate_image"
  - "generate_video"
  - "midjourney_*"
```

## Configuration

MCP mode uses the same config.yaml as CLI mode. Provider, API keys, and defaults are shared.

## Testing

List available tools without starting the server:

```bash
aigc-cli mcp --list-tools
```
