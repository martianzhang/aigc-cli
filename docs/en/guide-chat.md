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
| `generate_music` | Generate music |
| `midjourney_imagine` | Midjourney text-to-image |
| `search_ideas` | Search prompt ideas |
| `kb_find` | Search knowledge base |
| `kb_search` | Web search + save to KB |
| `detect_image` | AIGC detection |
| `remove_watermark` | Watermark removal |
| `convert_depth` | Image/video → grayscale depth map (offline, V2 models) |

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
| `--max-output` | Max response tokens | `4096` |
| `--context-size` | Max input context tokens; auto-compacts at 80% (summarizes older messages) | `0` (model default) |
| `--system` | System prompt | — |
| `--json` | Pass messages as JSON; forwarded **verbatim** when no flags are set, so unmodeled fields (e.g. `reasoning_effort`, vendor-private keys) reach the API. Explicitly-set flags override the matching key | — |
| `--dry-run` | Print the equivalent curl (real endpoint + body) without calling the API | — |

### Context Management

When `--context-size` is set, the conversation history is automatically summarized
(compacted) once it exceeds 80% of the limit, freeing space without manual action.
You can also type `/compact` in interactive mode to trigger compaction manually.

## JSON Mode

```bash
aigc-cli chat --json '{"messages": [{"role": "user", "content": "Hello"}]}'
```

`--json` also accepts **JSONC**: `//` and `/* */` comments, trailing commas, and a leading UTF-8 BOM are stripped before parsing. Stripping is string-literal aware, so a `//` in `"https://host/path"` is kept, and a comment-free strict JSON body is unchanged byte-for-byte.

With `--json` and no flags, the body is sent byte-for-byte unchanged. When you also pass flags, only the flags you explicitly set override the matching JSON key; every key you did not touch (including unmodeled vendor keys) is preserved exactly. Flag-to-key mapping follows the normal flag path (`--max-output` → `max_tokens`, `--no-stream` → `stream`, `--temperature` → `temperature`).

```bash
# JSON supplies the messages; the model and token cap come from flags
aigc-cli chat --json '{"messages": [{"role": "user", "content": "Hello"}]}' --model deepseek-v4-flash --max-output 2048
```

Here `messages` stays as written while `model` and `max_tokens` come from the flags. Behavioral flags (`--dry-run`, `--provider`, `--api-key`, `--api-base`, `--output`, `--verbose`, `--timeout`, `--context-size`, `--interactive`) never enter the body.

## Verbose Output

```bash
aigc-cli chat --message "Hello" --verbose
```

Shows token usage, timing, and cost information.
