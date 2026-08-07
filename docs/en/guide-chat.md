# AI Chat

Use `aigc-cli chat` for interactive multi-turn AI conversations with tool-calling support.

## Basic Usage

```bash
# Single message
aigc-cli chat --message "Hello, who are you?"

# Interactive REPL mode
aigc-cli chat
```

## Interactive REPL

Start the interactive chat:

```bash
aigc-cli chat
> Hello
> generate an image of a cat
```

The REPL supports:
- Multi-turn conversation
- Tool calling (image generation, video, Midjourney, ideas, knowledge base)
- Streaming output
- Command history

## Tool Calling

In REPL mode, the AI can use built-in tools:

| Tool | Description |
|---|---|
| `generate_image` | Generate images |
| `generate_video` | Generate videos |
| `midjourney_imagine` | Midjourney text-to-image |
| `search_ideas` | Search prompt ideas |
| `kb_find` | Search knowledge base |
| `kb_search` | Web search + save to KB |
| `detect_image` | AIGC detection |
| `remove_watermark` | Watermark removal |
| `convert_video_depth` | Video → grayscale depth video (offline) |

Tools can be enabled/disabled via config:

```yaml
# Only allow free/offline tools
tools_enable:
  - "search_ideas"
  - "detect_image"
  - "recognize_text"

# Block paid generation tools
tools_disable:
  - "generate_image"
  - "generate_video"
```

## Parameters

| Parameter | Description | Default |
|---|---|---|
| `--message` / `-m` | Single message (non-interactive) | — |
| `--model` | Model name | `deepseek-v4-flash` |
| `--temperature` | Response temperature | `0.7` |
| `--max-tokens` | Max response tokens | `4096` |
| `--system` | System prompt | — |
| `--json` | Pass messages as JSON | — |

## JSON Mode

```bash
aigc-cli chat --json '{"messages": [{"role": "user", "content": "Hello"}]}'
```

## Verbose Output

```bash
aigc-cli chat --message "Hello" --verbose
```

Shows token usage, timing, and cost information.
